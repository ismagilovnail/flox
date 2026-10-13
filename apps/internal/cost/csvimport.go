package cost

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// CSV import of one campaign's spend (§27-COST "CSV import per
// campaign/source/day/country"). The campaign comes from the route, never
// from the file. Columns, by header name in any order and case:
//
//	date               required, YYYY-MM-DD
//	amount             required, >= 0
//	currency           required, ISO 4217
//	country            optional, ISO 3166-1 alpha-2; empty = day total
//	traffic_source_id  optional
//
// An unknown column is an error rather than ignored: a misspelt "country"
// would otherwise land every row as a day total. The delimiter is "," or,
// when the header has ";" and no ",", ";" (a European spreadsheet export,
// whose amounts may then use a decimal comma). The import is all or
// nothing — one transaction, and any bad line rejects the file with an
// error per line.

// MaxImportRows bounds one file.
const MaxImportRows = 5000

// ImportLine is one parsed data line; Line is its 1-based line in the file
// (the header is line 1).
type ImportLine struct {
	Line int
	UpsertInput
}

// csvColumn names validateUpsert's fields as the file's columns.
var csvColumn = map[string]string{"entryDate": "date", "amount": "amount", "currency": "currency", "country": "country", "trafficSourceId": "traffic_source_id"}

var importColumns = map[string]bool{"date": true, "amount": true, "currency": true, "country": true, "traffic_source_id": true}

// ParseCSV reads the file into lines. Errors are keyed by "line N" (or
// "file" for the header and the file as a whole), the same map a
// validation error carries.
func ParseCSV(r io.Reader) ([]ImportLine, map[string]string) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, map[string]string{"file": "could not be read"}
	}
	text := strings.TrimPrefix(string(data), "\uFEFF") // Excel's UTF-8 BOM
	firstLine, _, _ := strings.Cut(text, "\n")
	delim := ','
	if strings.Contains(firstLine, ";") && !strings.Contains(firstLine, ",") {
		delim = ';'
	}

	cr := csv.NewReader(strings.NewReader(text))
	cr.Comma = delim
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if err != nil {
		return nil, map[string]string{"file": "empty, or not a CSV file"}
	}
	col := map[string]int{}
	for i, h := range header {
		name := strings.ToLower(strings.TrimSpace(h))
		if !importColumns[name] {
			return nil, map[string]string{"file": fmt.Sprintf("unknown column %q — want date, amount, currency, country, traffic_source_id", strings.TrimSpace(h))}
		}
		if _, dup := col[name]; dup {
			return nil, map[string]string{"file": fmt.Sprintf("column %q appears twice", name)}
		}
		col[name] = i
	}
	for _, need := range []string{"date", "amount", "currency"} {
		if _, ok := col[need]; !ok {
			return nil, map[string]string{"file": fmt.Sprintf("missing column %q", need)}
		}
	}

	var lines []ImportLine
	errs := map[string]string{}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				errs[fmt.Sprintf("line %d", pe.StartLine)] = "malformed CSV"
				return nil, errs
			}
			return nil, map[string]string{"file": "could not be read"}
		}
		lineNo, _ := cr.FieldPos(0)
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue // blank line
		}
		key := fmt.Sprintf("line %d", lineNo)
		if len(lines) == MaxImportRows {
			return nil, map[string]string{"file": fmt.Sprintf("more than %d rows — split the file", MaxImportRows)}
		}
		get := func(name string) string {
			i, ok := col[name]
			if !ok || i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}

		in := UpsertInput{Currency: get("currency"), Country: get("country")}
		day, err := time.Parse(dateLayout, get("date"))
		if err != nil {
			errs[key] = "date must be YYYY-MM-DD"
			continue
		}
		in.EntryDate = day
		amount := get("amount")
		if delim == ';' {
			amount = strings.Replace(amount, ",", ".", 1)
		}
		if in.Amount, err = strconv.ParseFloat(amount, 64); err != nil {
			errs[key] = "amount must be a number"
			continue
		}
		if src := get("traffic_source_id"); src != "" {
			in.TrafficSourceID = &src
		}
		// Field checks here too, so one pass names every bad line, not
		// only the unparsable ones.
		if msg := lineError(normalize(in)); msg != "" {
			errs[key] = msg
			continue
		}
		lines = append(lines, ImportLine{Line: lineNo, UpsertInput: in})
	}
	if len(errs) > 0 {
		return nil, errs
	}
	if len(lines) == 0 {
		return nil, map[string]string{"file": "no data rows"}
	}
	return lines, nil
}

// lineError is validateUpsert's verdict on one line, in the file's column
// names, or "".
func lineError(in UpsertInput) string {
	fields := validateUpsert(in)
	msgs := make([]string, 0, len(fields))
	for field, msg := range fields {
		msgs = append(msgs, csvColumn[field]+": "+msg)
	}
	sort.Strings(msgs)
	return strings.Join(msgs, "; ")
}

// Import writes every line as a manual entry of the campaign, in one
// transaction: all of them or none. It returns how many entries were
// written (new or overwritten).
func (s *Service) Import(ctx context.Context, orgID, campaignID string, lines []ImportLine) (int, error) {
	belongs, err := s.repo.CampaignBelongsToOrg(ctx, orgID, campaignID)
	if err != nil {
		return 0, fmt.Errorf("checking campaign: %w", err)
	}
	if !belongs {
		return 0, apierror.Validation("invalid cost import", map[string]string{"campaignId": "no such campaign in this organization"})
	}

	errs := map[string]string{}
	lineKey := func(l ImportLine) string { return fmt.Sprintf("line %d", l.Line) }
	seen := map[string]int{}   // source|day|country → line
	split := map[string]bool{} // source|day → whether by country
	sources := map[string]bool{}
	items := make([]batchItem, 0, len(lines))
	for _, l := range lines {
		in := normalize(l.UpsertInput)
		if msg := lineError(in); msg != "" {
			errs[lineKey(l)] = msg
			continue
		}
		srcKey := ""
		if in.TrafficSourceID != nil {
			srcKey = *in.TrafficSourceID
			sources[srcKey] = false
		}
		dayKey := srcKey + "|" + in.EntryDate.Format(dateLayout)
		if prev, dup := seen[dayKey+"|"+in.Country]; dup {
			errs[lineKey(l)] = fmt.Sprintf("same day and country as line %d", prev)
			continue
		}
		seen[dayKey+"|"+in.Country] = l.Line
		if bySplit, ok := split[dayKey]; ok && bySplit != (in.Country != "") {
			errs[lineKey(l)] = "this day has both a total and rows by country in the file — use one or the other"
			continue
		}
		split[dayKey] = in.Country != ""

		usd, ok, err := s.fx.ToUSD(ctx, in.Currency, in.Amount, in.EntryDate)
		if err != nil {
			return 0, fmt.Errorf("converting to usd: %w", err)
		}
		item := batchItem{id: idgen.New(), line: l.Line, in: in}
		if ok {
			item.amountUSD = &usd
		}
		items = append(items, item)
	}
	for src := range sources {
		ok, err := s.repo.TrafficSourceBelongsToOrg(ctx, orgID, src)
		if err != nil {
			return 0, fmt.Errorf("checking traffic source: %w", err)
		}
		if !ok {
			for _, it := range items {
				if it.in.TrafficSourceID != nil && *it.in.TrafficSourceID == src {
					errs[fmt.Sprintf("line %d", it.line)] = "no such traffic source in this organization"
				}
			}
		}
	}
	if len(errs) > 0 {
		return 0, apierror.Validation("invalid cost import", errs)
	}

	failed, err := s.repo.UpsertBatch(ctx, orgID, campaignID, items, SourceManual)
	if errors.Is(err, ErrSplitConflict) {
		return 0, apierror.Validation("invalid cost import", map[string]string{
			fmt.Sprintf("line %d", failed.line): "this day already has spend entered the other way (a total vs. by country) — delete the existing entries first",
		})
	}
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

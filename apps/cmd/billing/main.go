// Command billing shows and changes an organization's plan
// (internal/billing, docs/billing.md). Until a payment provider lands,
// this is how a plan is sold: the operator runs it after the client pays.
//
//	go run ./cmd/billing plans
//	go run ./cmd/billing show -org <organization id>
//	go run ./cmd/billing set-plan -org <organization id> -plan pro [-note "invoice 2026-031"]
//
// Needs DATABASE_URL. Every change closes the current plan, opens the new
// one and writes a billing.plan_changed row to the organization's audit log.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/billing"
	"github.com/ismagilovnail/flox/apps/internal/postgres"
)

const usage = `usage:
  billing plans
  billing show -org ID
  billing set-plan -org ID -plan CODE [-note TEXT]`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "billing:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	org := fs.String("org", "", "organization id")
	plan := fs.String("plan", "", "plan code (billing plans lists them)")
	note := fs.String("note", "", "why: an invoice, a deal")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pg, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pg.Close()
	repo := billing.NewRepository(pg)

	switch args[0] {
	case "plans":
		plans, err := repo.Plans(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("%-10s %-10s %12s %10s %8s %8s %10s\n", "CODE", "NAME", "CLICKS/MO", "CAMPAIGNS", "DOMAINS", "MEMBERS", "PRICE")
		for _, p := range plans {
			fmt.Printf("%-10s %-10s %12s %10s %8s %8s %7d.%02d %s\n", p.Code, p.Name,
				limit(p.MonthlyClicks), limit(p.MaxCampaigns), limit(p.MaxDomains), limit(p.MaxMembers),
				p.PriceCents/100, p.PriceCents%100, p.Currency)
		}
	case "show":
		if *org == "" {
			return errors.New("-org is required")
		}
		p, since, err := repo.Current(ctx, *org)
		if err != nil {
			return err
		}
		counts, err := repo.Counts(ctx, *org)
		if err != nil {
			return err
		}
		fmt.Printf("plan       %s (%s) since %s\n", p.Name, p.Code, since.UTC().Format(time.DateOnly))
		fmt.Printf("campaigns  %d / %s\ndomains    %d / %s\nmembers    %d / %s\n",
			counts.Campaigns, limit(p.MaxCampaigns), counts.Domains, limit(p.MaxDomains), counts.Members, limit(p.MaxMembers))
		history, err := repo.History(ctx, *org)
		if err != nil {
			return err
		}
		for _, c := range history {
			end := "now"
			if c.EndedAt != nil {
				end = c.EndedAt.UTC().Format(time.DateTime)
			}
			fmt.Printf("  %-10s %s – %s  %s\n", c.PlanCode, c.StartedAt.UTC().Format(time.DateTime), end, c.Note)
		}
	case "set-plan":
		if *org == "" || *plan == "" {
			return errors.New("-org and -plan are required")
		}
		if err := repo.SetPlan(ctx, *org, *plan, *note); err != nil {
			return err
		}
		fmt.Printf("organization %s is now on %s\n", *org, *plan)
	default:
		return errors.New(usage)
	}
	return nil
}

func limit[T int | int64](v *T) string {
	if v == nil {
		return "unlimited"
	}
	return fmt.Sprint(*v)
}

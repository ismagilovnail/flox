package integrations

import (
	"encoding/json"
	"fmt"
)

// encodePayload/decodePayload turn a provider's credential fields into
// the plaintext JSON blob that gets AES-256-GCM encrypted (encrypt) and
// stored as org_integrations.encrypted_payload — a flat JSON object of
// exactly the fields Provider.RequiredFields names, nothing else.
func encodePayload(fields map[string]string) (string, error) {
	b, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("integrations: encoding credential payload: %w", err)
	}
	return string(b), nil
}

func decodePayload(payload string) (map[string]string, error) {
	var fields map[string]string
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		return nil, fmt.Errorf("integrations: decoding credential payload: %w", err)
	}
	return fields, nil
}

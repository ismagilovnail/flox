package streamset

import "testing"

func TestNormalizeASN(t *testing.T) {
	for in, want := range map[string]string{
		"15169": "AS15169", "as15169": "AS15169", " AS15169 ": "AS15169", "AS15169": "AS15169",
		"AS": "AS", "": "", "Google": "Google", "AS12x": "AS12x",
	} {
		if got := normalizeASN(in); got != want {
			t.Errorf("normalizeASN(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeFilterNodeOnlyTouchesASNExactMatchOperators(t *testing.T) {
	root := FilterNode{Kind: NodeGroup, Joiner: "AND", Children: []FilterNode{
		{Kind: NodeCondition, Field: "asn", Operator: "IN", Value: "15169, as13238"},
		{Kind: NodeCondition, Field: "asn", Operator: "CONTAINS", Value: "151"},
		{Kind: NodeCondition, Field: "utm_source", Operator: "IS", Value: "15169"},
		{Kind: NodeGroup, Joiner: "OR", Children: []FilterNode{
			{Kind: NodeCondition, Field: "asn", Operator: "IS", Value: "15169"},
		}},
	}}
	normalizeFilterNode(&root)
	if got := root.Children[0].Value; got != "AS15169,AS13238" {
		t.Errorf("IN value = %q", got)
	}
	if got := root.Children[1].Value; got != "151" {
		t.Errorf("CONTAINS must stay literal, got %q", got)
	}
	if got := root.Children[2].Value; got != "15169" {
		t.Errorf("non-asn field must be untouched, got %q", got)
	}
	if got := root.Children[3].Children[0].Value; got != "AS15169" {
		t.Errorf("nested IS value = %q", got)
	}
}

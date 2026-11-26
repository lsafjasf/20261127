package experiment

import "testing"

func TestRunProducesAllStrategyRows(t *testing.T) {
	rows := Run(DefaultParams())
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Strategy]++
	}
	for _, st := range Strategies() {
		if counts[st.Name] == 0 {
			t.Fatalf("no metrics rows for strategy %s", st.Name)
		}
	}
}

func TestAggregateOrderingAndFinite(t *testing.T) {
	aggs := AggregateByStrategy(Run(DefaultParams()))
	if len(aggs) != 3 || aggs[0].Strategy != "linear_expand" {
		t.Fatalf("unexpected aggregates: %+v", aggs)
	}
	for _, a := range aggs {
		if a.MeanRMSE <= 0 || a.MeanNoiseR <= 0 {
			t.Fatalf("non-positive aggregate for %s: %+v", a.Strategy, a)
		}
	}
}

package main

import "testing"

func TestEnvOrDefault(t *testing.T) {
	const key = "SCRC_TEST_ENV"
	const fallback = "fallback"

	if got := envOrDefault(key, fallback); got != fallback {
		t.Fatalf("expected fallback when env unset, got %q", got)
	}

	t.Setenv(key, "value")
	if got := envOrDefault(key, fallback); got != "value" {
		t.Fatalf("expected env value, got %q", got)
	}
}

func TestParseBrokerList(t *testing.T) {
	input := " broker1:9092 , ,broker2:9093 ,"
	brokers := parseBrokerList(input)
	want := []string{"broker1:9092", "broker2:9093"}
	if len(brokers) != len(want) {
		t.Fatalf("expected %d brokers, got %d", len(want), len(brokers))
	}
	for i := range want {
		if brokers[i] != want[i] {
			t.Fatalf("unexpected broker at index %d: got %q want %q", i, brokers[i], want[i])
		}
	}
}

func TestParseMaxScripts(t *testing.T) {
	cases := map[string]int{
		"":   0,
		"-1": 0,
		"x":  0,
		"5":  5,
	}

	for input, want := range cases {
		if got := parseMaxScripts(input); got != want {
			t.Fatalf("parseMaxScripts(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestParseMaxParallel(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"", 1},
		{"not-a-number", 1},
		{"0", 1},
		{"-5", 1},
		{"3", 3},
	}

	for _, tc := range cases {
		if got := parseMaxParallel(tc.input); got != tc.want {
			t.Fatalf("parseMaxParallel(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

package main

import (
	"slices"
	"testing"
)

func TestSliceFlag_RepeatedFlags(t *testing.T) {
	var f sliceFlag
	if err := f.Set("zone"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := f.Set("timestamp"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"zone", "timestamp"}
	if !slices.Equal([]string(f), want) {
		t.Fatalf("got %v, want %v", []string(f), want)
	}
	if !slices.Contains(f, "zone") || !slices.Contains(f, "timestamp") {
		t.Fatalf("both algorithms must be present: %v", []string(f))
	}
}

func TestSliceFlag_CommaSeparated(t *testing.T) {
	var f sliceFlag
	if err := f.Set("zone, timestamp ,"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Comma-separated values are split and trimmed; empty entries dropped.
	want := []string{"zone", "timestamp"}
	if !slices.Equal([]string(f), want) {
		t.Fatalf("got %v, want %v", []string(f), want)
	}
}

func TestSliceFlag_Mixed(t *testing.T) {
	var f sliceFlag
	_ = f.Set("zone,timestamp")
	_ = f.Set("extra")

	want := []string{"zone", "timestamp", "extra"}
	if !slices.Equal([]string(f), want) {
		t.Fatalf("got %v, want %v", []string(f), want)
	}
}

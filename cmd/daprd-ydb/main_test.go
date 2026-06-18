package main

import "testing"

func TestResolveSocketFolderEnv(t *testing.T) {
	cases := []struct {
		name     string
		sdkVal   string
		runtimea string
		wantVal  string
		wantSet  bool
	}{
		{"neither set", "", "", "", false},
		{"only runtime set -> mirror", "", "/custom", "/custom", true},
		{"sdk already set -> keep", "/sdk", "/custom", "/sdk", false},
		{"only sdk set -> keep, no change", "/sdk", "", "/sdk", false},
		{"both equal -> no change", "/same", "/same", "/same", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, set := resolveSocketFolderEnv(c.sdkVal, c.runtimea)
			if got != c.wantVal || set != c.wantSet {
				t.Errorf("resolveSocketFolderEnv(%q, %q) = (%q, %v); want (%q, %v)",
					c.sdkVal, c.runtimea, got, set, c.wantVal, c.wantSet)
			}
		})
	}
}

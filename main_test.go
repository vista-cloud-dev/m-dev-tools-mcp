package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
)

func TestResolveBadProfile(t *testing.T) {
	_, _, err := opts{Profile: "bogus"}.resolve()
	if err == nil {
		t.Fatal("resolve: want error for unknown profile")
	}
}

func TestResolveMBinNotExecutable(t *testing.T) {
	f := filepath.Join(t.TempDir(), "notexec")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := opts{Profile: "default", MBin: f}.resolve()
	if err == nil {
		t.Errorf("resolve: a non-executable --m-bin should fail")
	}
}

func TestResolveMBinExecutable(t *testing.T) {
	f := filepath.Join(t.TempDir(), "m")
	if err := os.WriteFile(f, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	mbin, prof, err := opts{Profile: "safe", MBin: f}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if mbin != f || prof.Name != "safe" {
		t.Errorf("resolve = (%q, %q), want (%q, safe)", mbin, prof.Name, f)
	}
}

func TestCLIWiresServerCommands(t *testing.T) {
	cli := &CLI{}
	k, err := kong.New(cli, kong.Name("m-dev-tools-mcp"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.Parse([]string{"tools", "--profile", "safe"}); err != nil {
		t.Fatalf("parse tools: %v", err)
	}
	if cli.Tools.Profile != "safe" {
		t.Errorf("tools --profile not bound: %q", cli.Tools.Profile)
	}
}

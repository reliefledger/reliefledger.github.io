// Command reliefledger-site manages the Cloudflare edge for reliefledger.pro:
// the zone's DNS records (imported from dns-records.json) and the Worker that
// serves Markdown to agents that ask for it.
//
// The Cloudflare provider reads CLOUDFLARE_API_TOKEN from the environment.
// Nothing secret is stored in this public repository.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pulumi/pulumi-cloudflare/sdk/v5/go/cloudflare"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	workerName        = "reliefledger-markdown"
	workerSource      = "../edge/markdown-negotiation.mjs"
	recordsFile       = "dns-records.json"
	compatibilityDate = "2025-09-01"
)

// Record is one DNS record as exported by scripts/export-dns.sh.
type Record struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  bool   `json:"proxied"`
	Priority *int   `json:"priority,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		zoneName := config.New(ctx, "").Require("zoneName")
		zone, err := cloudflare.LookupZone(ctx, &cloudflare.LookupZoneArgs{Name: &zoneName})
		if err != nil {
			return fmt.Errorf("look up zone %s: %w", zoneName, err)
		}

		records, err := loadRecords(recordsFile)
		if err != nil {
			return err
		}
		if err := deployRecords(ctx, zone.Id, records); err != nil {
			return err
		}

		source, err := os.ReadFile(filepath.Clean(workerSource))
		if err != nil {
			return fmt.Errorf("read worker source: %w", err)
		}
		return deployWorker(ctx, zone.AccountId, zone.Id, zoneName, string(source))
	})
}

// loadRecords reads and validates the exported DNS records.
func loadRecords(path string) ([]Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s (run scripts/export-dns.sh first): %w", path, err)
	}
	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	seen := map[string]bool{}
	for _, r := range records {
		if len(r.ID) != 32 || r.Type == "" || r.Name == "" || r.Content == "" {
			return nil, fmt.Errorf("%s: record %+v needs a 32-character id, type, name and content", path, r)
		}
		if seen[r.ID] {
			return nil, fmt.Errorf("%s: duplicate record id %s", path, r.ID)
		}
		seen[r.ID] = true
	}
	return records, nil
}

// resourceName gives each record a stable logical name that survives
// content changes, so editing a value updates the record instead of
// replacing it.
func resourceName(r Record) string {
	name := strings.NewReplacer(".", "-", "_", "", "*", "wildcard").Replace(r.Name)
	return fmt.Sprintf("dns-%s-%s-%s", strings.ToLower(r.Type), name, r.ID[:8])
}

func deployRecords(ctx *pulumi.Context, zoneID string, records []Record) error {
	for _, r := range records {
		args := &cloudflare.RecordArgs{
			ZoneId:  pulumi.String(zoneID),
			Type:    pulumi.String(r.Type),
			Name:    pulumi.String(r.Name),
			Content: pulumi.String(r.Content),
			Ttl:     pulumi.Int(r.TTL),
			Proxied: pulumi.Bool(r.Proxied),
		}
		if r.Priority != nil {
			args.Priority = pulumi.IntPtr(*r.Priority)
		}
		if r.Comment != "" {
			args.Comment = pulumi.StringPtr(r.Comment)
		}
		// Import adopts the existing record; it is a no-op once in state.
		_, err := cloudflare.NewRecord(ctx, resourceName(r), args,
			pulumi.Import(pulumi.ID(zoneID+"/"+r.ID)), pulumi.Protect(true))
		if err != nil {
			return fmt.Errorf("record %s %s: %w", r.Type, r.Name, err)
		}
	}
	return nil
}

func deployWorker(ctx *pulumi.Context, accountID, zoneID, zoneName, source string) error {
	script, err := cloudflare.NewWorkersScript(ctx, workerName, &cloudflare.WorkersScriptArgs{
		AccountId:         pulumi.String(accountID),
		Name:              pulumi.String(workerName),
		Content:           pulumi.String(source),
		Module:            pulumi.Bool(true),
		CompatibilityDate: pulumi.String(compatibilityDate),
	})
	if err != nil {
		return fmt.Errorf("worker script: %w", err)
	}
	_, err = cloudflare.NewWorkersRoute(ctx, workerName+"-route", &cloudflare.WorkersRouteArgs{
		ZoneId:     pulumi.String(zoneID),
		Pattern:    pulumi.String(zoneName + "/*"),
		ScriptName: script.Name,
	})
	if err != nil {
		return fmt.Errorf("worker route: %w", err)
	}
	ctx.Export("workerName", script.Name)
	ctx.Export("workerRoute", pulumi.String(zoneName+"/*"))
	return nil
}

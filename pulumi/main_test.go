package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const testID = "0123456789abcdef0123456789abcdef"

func writeRecords(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "records.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRecords(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"valid", `[{"id":"` + testID + `","type":"TXT","name":"_dmarc.reliefledger.pro","content":"v=DMARC1; p=none","ttl":1}]`, ""},
		{"short id", `[{"id":"abc","type":"A","name":"x","content":"1.2.3.4","ttl":1}]`, "32-character id"},
		{"missing content", `[{"id":"` + testID + `","type":"A","name":"x","ttl":1}]`, "32-character id"},
		{"duplicate", `[{"id":"` + testID + `","type":"A","name":"x","content":"1","ttl":1},{"id":"` + testID + `","type":"A","name":"y","content":"2","ttl":1}]`, "duplicate"},
		{"bad json", `{`, "parse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadRecords(writeRecords(t, tt.body))
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadRecordsMissingFileExplainsNextStep(t *testing.T) {
	_, err := loadRecords(filepath.Join(t.TempDir(), "none.json"))
	if err == nil || !strings.Contains(err.Error(), "export-dns.sh") {
		t.Fatalf("error = %v, want hint to run export-dns.sh", err)
	}
}

func TestResourceNameIsStableAcrossContentChanges(t *testing.T) {
	a := Record{ID: testID, Type: "TXT", Name: "_dmarc.reliefledger.pro", Content: "v=DMARC1; p=none"}
	b := a
	b.Content = "v=DMARC1; p=quarantine"
	if resourceName(a) != resourceName(b) {
		t.Fatalf("name changed with content: %s vs %s", resourceName(a), resourceName(b))
	}
	if got, want := resourceName(a), "dns-txt-dmarc-reliefledger-pro-01234567"; got != want {
		t.Fatalf("resourceName = %s, want %s", got, want)
	}
}

// mocks records every resource the program registers.
type mocks struct {
	mu        sync.Mutex
	resources map[string]resource.PropertyMap
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources[args.TypeToken+"::"+args.Name] = args.Inputs
	return args.Name + "-id", args.Inputs, nil
}

func (m *mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func TestDeployRegistersRecordsWorkerAndRoute(t *testing.T) {
	m := &mocks{resources: map[string]resource.PropertyMap{}}
	priority := 1
	records := []Record{
		{ID: testID, Type: "MX", Name: "reliefledger.pro", Content: "aspmx.l.google.com", TTL: 1, Priority: &priority},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		if err := deployRecords(ctx, "zone123", records); err != nil {
			return err
		}
		return deployWorker(ctx, "acct456", "zone123", "reliefledger.pro", "export default {}")
	}, pulumi.WithMocks("reliefledger-site", "test", m))
	if err != nil {
		t.Fatal(err)
	}

	rec, ok := m.resources["cloudflare:index/record:Record::dns-mx-reliefledger-pro-01234567"]
	if !ok {
		t.Fatalf("record not registered; got %v", keys(m.resources))
	}
	if rec["priority"].NumberValue() != 1 || rec["zoneId"].StringValue() != "zone123" {
		t.Fatalf("record inputs = %v", rec)
	}

	script, ok := m.resources["cloudflare:index/workersScript:WorkersScript::reliefledger-markdown"]
	if !ok {
		t.Fatalf("worker not registered; got %v", keys(m.resources))
	}
	if !script["module"].BoolValue() || script["accountId"].StringValue() != "acct456" {
		t.Fatalf("worker inputs = %v", script)
	}

	route, ok := m.resources["cloudflare:index/workersRoute:WorkersRoute::reliefledger-markdown-route"]
	if !ok {
		t.Fatalf("route not registered; got %v", keys(m.resources))
	}
	if route["pattern"].StringValue() != "reliefledger.pro/*" {
		t.Fatalf("route pattern = %v", route["pattern"])
	}
}

func TestWorkerSourceIsTheTestedModule(t *testing.T) {
	src, err := os.ReadFile(workerSource)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "export default") || !strings.Contains(string(src), "safeHandle") {
		t.Fatal("worker source must export the fail-open default handler")
	}
}

func keys(m map[string]resource.PropertyMap) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

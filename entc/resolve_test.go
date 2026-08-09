package entc

import (
	"strings"
	"testing"
)

const testDescriptorSetPath = "../proto/descriptorset.binpb"

func TestSplitProcedure(t *testing.T) {
	tests := []struct {
		name        string
		procedure   string
		wantService string
		wantMethod  string
		wantErr     bool
	}{
		{
			name:        "leading slash",
			procedure:   "/entconnecttest.v1.OrderReadService/GetOrder",
			wantService: "entconnecttest.v1.OrderReadService",
			wantMethod:  "GetOrder",
		},
		{
			name:        "no leading slash",
			procedure:   "entconnecttest.v1.OrderReadService/GetOrder",
			wantService: "entconnecttest.v1.OrderReadService",
			wantMethod:  "GetOrder",
		},
		{
			name:      "malformed: no separator",
			procedure: "entconnecttest.v1.OrderReadServiceGetOrder",
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, method, err := SplitProcedure(tt.procedure)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if svc != tt.wantService || method != tt.wantMethod {
				t.Fatalf("want (%q, %q), got (%q, %q)", tt.wantService, tt.wantMethod, svc, method)
			}
		})
	}
}

func TestResolveMethod(t *testing.T) {
	files, err := LoadDescriptorSet(testDescriptorSetPath)
	if err != nil {
		t.Fatalf("LoadDescriptorSet: %v", err)
	}

	t.Run("resolvable procedure", func(t *testing.T) {
		m, err := ResolveMethod(files, "/entconnecttest.v1.OrderReadService/GetOrder", testDescriptorSetPath)
		if err != nil {
			t.Fatalf("ResolveMethod: %v", err)
		}
		if string(m.Name()) != "GetOrder" {
			t.Fatalf("want method name %q, got %q", "GetOrder", m.Name())
		}
	})

	t.Run("malformed procedure", func(t *testing.T) {
		_, err := ResolveMethod(files, "no-slash-here", testDescriptorSetPath)
		if err == nil {
			t.Fatal("want an error for a malformed procedure, got nil")
		}
	})

	t.Run("unknown service", func(t *testing.T) {
		_, err := ResolveMethod(files, "/entconnecttest.v1.NoSuchService/GetOrder", testDescriptorSetPath)
		if err == nil {
			t.Fatal("want an error for an unknown service, got nil")
		}
		if !strings.Contains(err.Error(), testDescriptorSetPath) {
			t.Fatalf("want the error to name the descriptor set path %q, got: %v", testDescriptorSetPath, err)
		}
	})

	t.Run("unknown method", func(t *testing.T) {
		_, err := ResolveMethod(files, "/entconnecttest.v1.OrderReadService/NoSuchMethod", testDescriptorSetPath)
		if err == nil {
			t.Fatal("want an error for an unknown method, got nil")
		}
		if !strings.Contains(err.Error(), testDescriptorSetPath) {
			t.Fatalf("want the error to name the descriptor set path %q, got: %v", testDescriptorSetPath, err)
		}
	})
}

func TestAllProcedures(t *testing.T) {
	files, err := LoadDescriptorSet(testDescriptorSetPath)
	if err != nil {
		t.Fatalf("LoadDescriptorSet: %v", err)
	}
	procs := AllProcedures(files)
	found := false
	for _, p := range procs {
		if p == "/entconnecttest.v1.OrderReadService/GetOrder" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want %q in AllProcedures(), got: %v", "/entconnecttest.v1.OrderReadService/GetOrder", procs)
	}
	sorted := append([]string(nil), procs...)
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1] > sorted[i] {
			t.Fatalf("AllProcedures() is not sorted: %v", procs)
		}
	}
}

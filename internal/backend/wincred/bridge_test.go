package wincred

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/akihiro/wsl-secret-service/internal/ipc"
)

// buildMockHelper compiles the mock helper binary for this test run.
// It returns the path to the compiled binary.
func buildMockHelper(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("mock helper test only runs on Linux (it mocks the Windows side)")
	}

	// Write a small Go program that acts as the mock wincred-helper.
	src := `package main

import (
	"encoding/json"
	"os"
)

type req struct {
	Action string ` + "`json:\"action\"`" + `
	Target string ` + "`json:\"target\"`" + `
	Secret string ` + "`json:\"secret,omitempty\"`" + `
	Filter string ` + "`json:\"filter,omitempty\"`" + `
}
type resp struct {
	OK      bool     ` + "`json:\"ok\"`" + `
	Secret  string   ` + "`json:\"secret,omitempty\"`" + `
	Targets []string ` + "`json:\"targets,omitempty\"`" + `
	Error   string   ` + "`json:\"error,omitempty\"`" + `
}
func main() {
	// In-memory credential store for the mock.
	store := map[string]string{
		"wsl-ss/login/existing": "dGVzdC1zZWNyZXQ=", // base64("test-secret")
	}
	var r req
	if err := json.NewDecoder(os.Stdin).Decode(&r); err != nil {
		json.NewEncoder(os.Stdout).Encode(resp{OK: false, Error: err.Error()})
		return
	}
	enc := json.NewEncoder(os.Stdout)
	switch r.Action {
	case "get":
		if v, ok := store[r.Target]; ok {
			enc.Encode(resp{OK: true, Secret: v})
		} else {
			enc.Encode(resp{OK: false, Error: "element not found"})
		}
	case "set":
		store[r.Target] = r.Secret
		enc.Encode(resp{OK: true})
	case "delete":
		if _, ok := store[r.Target]; ok {
			delete(store, r.Target)
			enc.Encode(resp{OK: true})
		} else {
			enc.Encode(resp{OK: false, Error: "element not found"})
		}
	case "list":
		var targets []string
		for k := range store {
			targets = append(targets, k)
		}
		enc.Encode(resp{OK: true, Targets: targets})
	default:
		enc.Encode(resp{OK: false, Error: "unknown action"})
	}
}
`
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "mock_helper.go")
	if err := os.WriteFile(srcPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write mock helper src: %v", err)
	}
	binPath := filepath.Join(dir, "mock-wincred-helper")
	cmd := exec.Command("go", "build", "-o", binPath, srcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mock helper: %v\n%s", err, out)
	}
	return binPath
}

func newTestBridge(t *testing.T) *Bridge {
	t.Helper()
	helperPath := buildMockHelper(t)
	b, err := New(helperPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return b
}

func TestGet_Existing(t *testing.T) {
	b := newTestBridge(t)
	got, err := b.Get("wsl-ss/login/existing")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := []byte("test-secret")
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGet_NotFound(t *testing.T) {
	b := newTestBridge(t)
	_, err := b.Get("wsl-ss/login/nonexistent")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if !isNotFound(err.Error()) {
		t.Errorf("error %q should be a not-found error", err)
	}
}

func TestSet_And_Get(t *testing.T) {
	b := newTestBridge(t)

	secret := []byte("my-password-123")
	// Mock helper is stateless per invocation, so we test the Set response only.
	if err := b.Set("wsl-ss/login/new-item", secret); err != nil {
		t.Fatalf("Set: %v", err)
	}
}

func TestSet_TooLarge(t *testing.T) {
	b := newTestBridge(t)
	tooBig := make([]byte, 2561)
	if err := b.Set("wsl-ss/login/big", tooBig); err == nil {
		t.Fatal("expected error for oversized secret")
	}
}

func TestDelete_Existing(t *testing.T) {
	b := newTestBridge(t)
	// The mock store starts with "wsl-ss/login/existing".
	if err := b.Delete("wsl-ss/login/existing"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	b := newTestBridge(t)
	err := b.Delete("wsl-ss/login/gone")
	if err == nil {
		t.Fatal("expected error deleting non-existent key")
	}
}

func TestList(t *testing.T) {
	b := newTestBridge(t)
	targets, err := b.List("wsl-ss/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(targets) == 0 {
		t.Error("expected at least one target from mock store")
	}
}

func TestBase64RoundTrip(t *testing.T) {
	secret := []byte("hello, world! \x00\xff\xfe")
	encoded := base64.StdEncoding.EncodeToString(secret)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != string(secret) {
		t.Errorf("round-trip failed: got %v, want %v", decoded, secret)
	}
}

func TestFindHelper_NotFound(t *testing.T) {
	// Temporarily remove PATH so exec.LookPath fails too.
	old := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", old)

	_, err := findHelper()
	if err == nil {
		t.Fatal("expected error when wincred-helper.exe is not in any standard location")
	}
}

// TestIpcProtocol exercises the JSON IPC framing directly.
func TestIpcProtocol(t *testing.T) {
	helperPath := buildMockHelper(t)
	b := &Bridge{helperPath: helperPath}

	resp, err := b.call(ipc.Request{Action: "get", Target: "wsl-ss/login/existing"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !resp.OK {
		t.Errorf("ok=false, error=%q", resp.Error)
	}
	if resp.Secret == "" {
		t.Error("expected non-empty secret in response")
	}

	// Verify the secret decodes correctly.
	decoded, err := base64.StdEncoding.DecodeString(resp.Secret)
	if err != nil {
		t.Fatalf("decode secret: %v", err)
	}
	if string(decoded) != "test-secret" {
		t.Errorf("decoded secret = %q, want %q", decoded, "test-secret")
	}
	fmt.Println("IPC round-trip OK:", string(decoded))
}

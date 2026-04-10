package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestPrintTable(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf

	PrintTable([]string{"NAME", "STATUS"}, [][]string{
		{"exp-1", "Running"},
		{"exp-2", "Completed"},
	})

	out := buf.String()
	if !contains(out, "NAME") || !contains(out, "STATUS") {
		t.Errorf("table missing headers: %s", out)
	}
	if !contains(out, "exp-1") || !contains(out, "exp-2") {
		t.Errorf("table missing rows: %s", out)
	}
}

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf

	obj := map[string]string{"name": "test", "status": "ok"}
	if err := PrintJSON(obj); err != nil {
		t.Fatal(err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if parsed["name"] != "test" {
		t.Errorf("expected name=test, got %s", parsed["name"])
	}
}

func TestPrintYAML(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf

	obj := map[string]string{"name": "test"}
	if err := PrintYAML(obj); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !contains(out, "name: test") {
		t.Errorf("expected YAML with 'name: test', got: %s", out)
	}
}

func TestPrintObject_Table(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf

	err := PrintObject("table", nil, []string{"A", "B"}, [][]string{{"1", "2"}})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(buf.String(), "A") {
		t.Error("expected table output")
	}
}

func TestPrintObject_JSON(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf

	err := PrintObject("json", map[string]int{"x": 1}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(buf.String(), `"x": 1`) {
		t.Errorf("expected JSON output, got: %s", buf.String())
	}
}

func TestPrintObject_YAML(t *testing.T) {
	var buf bytes.Buffer
	Output = &buf

	err := PrintObject("yaml", map[string]int{"x": 1}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(buf.String(), "x: 1") {
		t.Errorf("expected YAML output, got: %s", buf.String())
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

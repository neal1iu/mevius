package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mevius/internal/domain"
	"mevius/internal/provider"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkflowWriteCannotBorrowAnotherWorkflowRun(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("write escaped workflow boundary")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":99,"workflow_id":123}`))
	}))
	defer remote.Close()
	p := NewProvider()
	cfg, _ := json.Marshal(pipelineConfig{RepositoryExternalID: "acme/app", WorkflowID: 42})
	r := &domain.ResourceInstance{ExternalID: "acme/app#42", ProviderConfig: cfg}
	conn := &domain.ProviderConnection{Endpoint: remote.URL}
	e := p.actions.CancelPipelineRun(context.Background(), conn, []byte("credential"), r, "99")
	var pe *provider.Error
	if !errors.As(e, &pe) || pe.Kind != provider.KindUnsupported {
		t.Fatal("different workflow accepted", e)
	}
	if _, e = p.actions.RerunPipeline(context.Background(), conn, []byte("credential"), r, "99"); e == nil {
		t.Fatal("different workflow rerun accepted")
	}
}
func TestLogArchiveDecompressionBounded(t *testing.T) {
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	file, e := w.Create("large.log")
	if e != nil {
		t.Fatal(e)
	}
	file.Write(bytes.Repeat([]byte("x"), 16*1024*1024))
	w.Close()
	content, truncated := tailZipContent(data.Bytes(), 1024)
	if len(content) > 1024 || !truncated {
		t.Fatal("unbounded log projection")
	}
}

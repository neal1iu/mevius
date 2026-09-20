package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"mevius/internal/domain"
	"mevius/internal/service"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, statusFor(err), map[string]any{"error": err.Error()})
}
func decodeBody(r *http.Request, value any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func (s *server) catalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"providers": s.registry.Descriptors()})
}
func (s *server) oauthProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := s.oauth.Providers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
}
func (s *server) saveOAuthConfiguration(w http.ResponseWriter, r *http.Request) {
	var input service.OAuthClientConfigurationInput
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.oauth.SaveConfiguration(r.Context(), domain.ProviderID(chi.URLParam(r, "providerID")), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) deleteOAuthConfiguration(w http.ResponseWriter, r *http.Request) {
	if err := s.oauth.DeleteConfiguration(r.Context(), domain.ProviderID(chi.URLParam(r, "providerID"))); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) oauthStart(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ProviderID domain.ProviderID `json:"provider_id"`
		Endpoint   string            `json:"endpoint,omitempty"`
	}
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.oauth.Start(r.Context(), input.ProviderID, input.Endpoint)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	providerID := domain.ProviderID(chi.URLParam(r, "providerID"))
	sessionID, err := s.oauth.Callback(r.Context(), providerID, r.URL.Query().Get("state"), r.URL.Query().Get("code"), r.URL.Query().Get("error"))
	http.Redirect(w, r, s.oauth.FrontendReturnURL(r.Context(), sessionID, err), http.StatusSeeOther)
}
func (s *server) oauthSession(w http.ResponseWriter, r *http.Request) {
	result, err := s.oauth.GetSession(r.Context(), chi.URLParam(r, "sessionID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) oauthComplete(w http.ResponseWriter, r *http.Request) {
	var input service.CompleteOAuthInput
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.oauth.Complete(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *server) probeConnection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ProviderID domain.ProviderID `json:"provider_id"`
		Endpoint   string            `json:"endpoint"`
		Credential string            `json:"credential"`
	}
	if err := decodeBody(r, &input); err != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.connections.Probe(r.Context(), input.ProviderID, input.Endpoint, []byte(input.Credential))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) createConnection(w http.ResponseWriter, r *http.Request) {
	var input service.CreateConnectionInput
	if err := decodeBody(r, &input); err != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.connections.Create(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *server) listConnections(w http.ResponseWriter, r *http.Request) {
	result, err := s.connections.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) getConnection(w http.ResponseWriter, r *http.Request) {
	result, err := s.connections.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) rotateCredential(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Credential string `json:"credential"`
	}
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.connections.RotateCredential(r.Context(), chi.URLParam(r, "id"), input.Credential)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) deleteConnection(w http.ResponseWriter, r *http.Request) {
	if err := s.connections.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) discover(w http.ResponseWriter, r *http.Request) {
	result, err := s.resources.Discover(r.Context(), chi.URLParam(r, "id"), domain.ProductID(chi.URLParam(r, "productID")), r.URL.Query().Get("parent_instance_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) listProjects(w http.ResponseWriter, r *http.Request) {
	result, err := s.projects.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) createProject(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.projects.Create(r.Context(), input.Name, input.Description)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *server) getProject(w http.ResponseWriter, r *http.Request) {
	result, err := s.projects.GetDetail(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) updateProject(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.projects.Update(r.Context(), chi.URLParam(r, "id"), input.Name, input.Description)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) deleteProject(w http.ResponseWriter, r *http.Request) {
	if err := s.projects.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) listProjectResources(w http.ResponseWriter, r *http.Request) {
	result, err := s.links.ListProjectResources(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) attachProjectResource(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ResourceInstanceID string `json:"resource_instance_id"`
		Alias              string `json:"alias"`
		Purpose            string `json:"purpose"`
	}
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.links.Attach(r.Context(), chi.URLParam(r, "id"), input.ResourceInstanceID, input.Alias, input.Purpose)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *server) updateProjectResource(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Alias   string `json:"alias"`
		Purpose string `json:"purpose"`
	}
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.links.UpdateProjectResource(r.Context(), chi.URLParam(r, "resourceID"), input.Alias, input.Purpose)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) detachProjectResource(w http.ResponseWriter, r *http.Request) {
	if err := s.links.Detach(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "resourceID")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listResources(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result, err := s.resources.List(r.Context(), service.ResourceFilter{ConnectionID: q.Get("connection"), ProductID: domain.ProductID(q.Get("product")), Kind: domain.ResourceKind(q.Get("kind")), Lifecycle: domain.LifecycleMode(q.Get("lifecycle")), SyncStatus: domain.SyncStatus(q.Get("sync_status"))})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) getResource(w http.ResponseWriter, r *http.Request) {
	result, err := s.resources.Get(r.Context(), chi.URLParam(r, "id"), true)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) importResource(w http.ResponseWriter, r *http.Request) {
	var input service.ImportResourceInput
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.resources.Import(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *server) createResource(w http.ResponseWriter, r *http.Request) {
	var input service.CreateResourceInput
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.resources.Create(r.Context(), input, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *server) refreshResource(w http.ResponseWriter, r *http.Request) {
	result, err := s.resources.Refresh(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) releaseResource(w http.ResponseWriter, r *http.Request) {
	result, err := s.resources.Release(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) forgetResource(w http.ResponseWriter, r *http.Request) {
	if err := s.resources.Forget(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) deleteRemote(w http.ResponseWriter, r *http.Request) {
	if err := s.resources.DeleteRemote(r.Context(), chi.URLParam(r, "id"), r.Header.Get("Idempotency-Key")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listRelations(w http.ResponseWriter, r *http.Request) {
	result, err := s.links.ListRelations(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) createRelation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		From   string              `json:"from_resource_instance_id"`
		To     string              `json:"to_resource_instance_id"`
		Type   domain.RelationType `json:"relation_type"`
		Config any                 `json:"config"`
	}
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.links.CreateRelation(r.Context(), input.From, input.To, input.Type, domain.RelationUser, input.Config)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *server) deleteRelation(w http.ResponseWriter, r *http.Request) {
	if err := s.links.DeleteRelation(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listDeployments(w http.ResponseWriter, r *http.Request) {
	result, err := s.runtime.ListDeployments(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) triggerDeployment(w http.ResponseWriter, r *http.Request) {
	result, err := s.runtime.TriggerDeployment(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}
func (s *server) deploymentLogs(w http.ResponseWriter, r *http.Request) {
	s.logs(w, r, chi.URLParam(r, "executionID"))
}
func (s *server) triggerPipeline(w http.ResponseWriter, r *http.Request) {
	var input domain.TriggerPipelineRequest
	if r.ContentLength > 0 && decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.runtime.TriggerPipeline(r.Context(), chi.URLParam(r, "id"), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}
func (s *server) listPipelineRuns(w http.ResponseWriter, r *http.Request) {
	result, err := s.runtime.ListPipelineRuns(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) getPipelineRun(w http.ResponseWriter, r *http.Request) {
	result, err := s.runtime.GetPipelineRun(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "runID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) cancelPipeline(w http.ResponseWriter, r *http.Request) {
	_, err := s.runtime.PipelineAction(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "runID"), "cancel")
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
func (s *server) rerunPipeline(w http.ResponseWriter, r *http.Request) {
	result, err := s.runtime.PipelineAction(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "runID"), "rerun")
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}
func (s *server) pipelineLogs(w http.ResponseWriter, r *http.Request) {
	s.logs(w, r, chi.URLParam(r, "runID"))
}
func (s *server) logs(w http.ResponseWriter, r *http.Request, executionID string) {
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	result, err := s.runtime.Logs(r.Context(), chi.URLParam(r, "id"), executionID, tail)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) listDNS(w http.ResponseWriter, r *http.Request) {
	result, err := s.runtime.ListDNS(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) createDNS(w http.ResponseWriter, r *http.Request) {
	var input domain.DNSRecord
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.runtime.CreateDNS(r.Context(), chi.URLParam(r, "id"), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *server) updateDNS(w http.ResponseWriter, r *http.Request) {
	var input domain.DNSRecord
	if decodeBody(r, &input) != nil {
		writeError(w, service.ErrInvalid)
		return
	}
	result, err := s.runtime.UpdateDNS(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "recordID"), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *server) deleteDNS(w http.ResponseWriter, r *http.Request) {
	if err := s.runtime.DeleteDNS(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "recordID")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

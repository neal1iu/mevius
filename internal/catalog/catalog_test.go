package catalog_test

import (
	"context"
	"errors"
	"mevius/internal/catalog"
	fxt "mevius/internal/catalogtest"
	"strings"
	"sync"
	"testing"
)

var ctx = context.Background()

func TestIdentityCanonicalization(t *testing.T) {
	a, e := catalog.IdentityKey([]string{"a/b", "c"})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := catalog.IdentityKey([]string{"a", "b/c"})
	if a == b {
		t.Fatal("delimiter collision")
	}
	if _, e = catalog.IdentityKey([]string{""}); e == nil {
		t.Fatal("empty identity accepted")
	}
}
func TestMultiCredentialIdentityAndIsolation(t *testing.T) {
	f := fxt.New(t)
	i := f.Instance(t, "public")
	read := f.Connection(t, i.ID, "read")
	write := f.Connection(t, i.ID, "write")
	br := f.Bind(t, read, "A", "live")
	bw := f.Bind(t, write, "B", "live")
	f.Put("fixture.project", "repo", []string{"stable-123"})
	r := f.Import(t, br, "fixture.project", "repo")
	r2 := f.Import(t, bw, "fixture.project", "repo")
	if r.ID != r2.ID {
		t.Fatal("connection leaked into identity")
	}
	accesses, e := f.Service.Accesses(ctx, r.ID)
	if e != nil || len(accesses) != 2 {
		t.Fatalf("accesses: %v %v", accesses, e)
	}
	var ra, wa string
	for _, a := range accesses {
		if a.BindingID == br.ID {
			ra = a.ID
		} else {
			wa = a.ID
		}
	}
	input := catalog.OperationInput{TypeID: r.TypeID, ActionID: "update", TargetKind: "resource", TargetID: r.ID, Input: fxt.Doc(map[string]any{})}
	if _, e = f.Service.Submit(ctx, "ambiguous", input); !errors.Is(e, catalog.ErrConflict) {
		t.Fatalf("expected explicit selection: %v", e)
	}
	input.AccessID = ra
	if _, e = f.Service.Submit(ctx, "read-write", input); !errors.Is(e, catalog.ErrForbidden) {
		t.Fatalf("read path: %v", e)
	}
	input.AccessID = wa
	op, e := f.Service.Submit(ctx, "write", input)
	if e != nil || op.Status != "succeeded" {
		t.Fatalf("write: %v %v", op, e)
	}
	if f.Calls["update"] != 1 {
		t.Fatal("unexpected credential fallback")
	}
	p, e := f.Service.SaveProject(ctx, "", "app", "")
	if e != nil {
		t.Fatal(e)
	}
	for _, alias := range []string{"a", "b"} {
		if _, e = f.Service.Attach(ctx, catalog.ProjectResource{ProjectID: p.ID, ResourceID: r.ID, Alias: alias, Role: "future_role"}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = f.Service.RotateCredential(ctx, write.ID, []byte("new")); e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.Service.SelectAccess(ctx, r, wa); !errors.Is(e, catalog.ErrForbidden) {
		t.Fatalf("old permissions survived rotation: %v", e)
	}
	if e = f.Service.DeleteConnection(ctx, write.ID); e != nil {
		t.Fatal(e)
	}
	if e = f.Service.DeleteConnection(ctx, read.ID); e != nil {
		t.Fatal(e)
	}
	got, e := f.Service.Resource(ctx, r.ID)
	if e != nil || got.ID != r.ID {
		t.Fatal("authorization removal deleted inventory")
	}
	links, e := f.Service.ProjectResources(ctx, p.ID)
	if e != nil || len(links) != 2 {
		t.Fatal("attachments lost")
	}
}
func TestNamespaceAndNaturalIdentity(t *testing.T) {
	f := fxt.New(t)
	f.Put("fixture.webhook", "same", []string{"same"})
	ids := map[string]bool{}
	for _, key := range []string{"cloud", "enterprise"} {
		i := f.Instance(t, key)
		c := f.Connection(t, i.ID, "write")
		for _, ns := range [][2]string{{"A", "live"}, {"A", "sandbox"}, {"B", "live"}} {
			b := f.Bind(t, c, ns[0], ns[1])
			r := f.Import(t, b, "fixture.webhook", "same")
			if ids[r.ID] {
				t.Fatal("namespace collision")
			}
			ids[r.ID] = true
		}
	}
	if len(ids) != 6 {
		t.Fatal(ids)
	}
}
func TestUnimportedParentAndReferenceResolution(t *testing.T) {
	f := fxt.New(t)
	i := f.Instance(t, "public")
	c := f.Connection(t, i.ID, "write")
	b := f.Bind(t, c, "A", "live")
	parent := catalog.ObservedRelation{Type: "parent", Reference: fxt.Reference(i.ID, "fixture.project", "parent", []string{"p1"})}
	f.Put("fixture.branch", "child", []string{"p1", "b1"}, parent)
	child := f.Import(t, b, "fixture.branch", "child")
	rels, e := f.Service.Relations(ctx, child.ID)
	if e != nil || len(rels) != 1 || rels[0].Reference.ResolvedID != nil {
		t.Fatalf("fake parent: %v %v", rels, e)
	}
	p, e := f.Service.SaveProject(ctx, "", "child-only", "")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Service.Attach(ctx, catalog.ProjectResource{ProjectID: p.ID, ResourceID: child.ID, Alias: "branch", Role: "future_role"}); e != nil {
		t.Fatal(e)
	}
	f.Put("fixture.project", "parent", []string{"p1"})
	real := f.Import(t, b, "fixture.project", "parent")
	rels, e = f.Service.Relations(ctx, child.ID)
	if e != nil || rels[0].Reference.ResolvedID == nil || *rels[0].Reference.ResolvedID != real.ID {
		t.Fatal("verified reference not resolved")
	}
	if e = f.Service.Forget(ctx, real.ID); !errors.Is(e, catalog.ErrConflict) {
		t.Fatalf("blocking parent: %v", e)
	}
	f.Put("fixture.webhook", "hook", []string{"hook"}, catalog.ObservedRelation{Type: "source_repo", Reference: fxt.Reference(i.ID, "fixture.project", "parent", nil)})
	hook := f.Import(t, b, "fixture.webhook", "hook")
	rels, e = f.Service.Relations(ctx, hook.ID)
	if e != nil || rels[0].Reference.ResolvedID != nil {
		t.Fatal("locator-only reference inferred as UID")
	}
}
func TestPathFailureAndPartialRelations(t *testing.T) {
	f := fxt.New(t)
	i := f.Instance(t, "public")
	c1 := f.Connection(t, i.ID, "write")
	c2 := f.Connection(t, i.ID, "read")
	b1 := f.Bind(t, c1, "A", "live")
	b2 := f.Bind(t, c2, "A", "live")
	ref := catalog.ObservedRelation{Type: "source_repo", Reference: fxt.Reference(i.ID, "fixture.project", "repo", nil)}
	f.Put("fixture.webhook", "hook", []string{"hook"}, ref)
	r := f.Import(t, b1, "fixture.webhook", "hook")
	if _, e := f.Service.ImportWithConfirmation(ctx, b2.ID, "fixture.webhook", fxt.Doc(map[string]any{"id": "hook"}), r.ID); e != nil {
		t.Fatal(e)
	}
	a, _ := f.Service.Accesses(ctx, r.ID)
	var id1, id2 string
	for _, v := range a {
		if v.BindingID == b1.ID {
			id1 = v.ID
		} else {
			id2 = v.ID
		}
	}
	f.Failures[c1.ID] = true
	if _, e := f.Service.Refresh(ctx, r.ID, id1); e == nil || strings.Contains(e.Error(), "secret") {
		t.Fatal("raw failure exposed")
	}
	a, _ = f.Service.Accesses(ctx, r.ID)
	for _, v := range a {
		if v.ID == id2 && (v.ErrorCode != "" || v.LastSuccess == nil) {
			t.Fatal("failed path overwrote healthy observation")
		}
	}
	f.Failures[c1.ID] = false
	f.Put("fixture.webhook", "hook", []string{"hook"})
	if _, e := f.Service.Refresh(ctx, r.ID, id1); e != nil {
		t.Fatal(e)
	}
	rels, _ := f.Service.Relations(ctx, r.ID)
	if len(rels) != 2 {
		t.Fatal("partial observation erased known relations")
	}
	v := f.Observations["fixture.webhook:hook"]
	v.CompleteRelations = []string{"source_repo"}
	f.Observations["fixture.webhook:hook"] = v
	if _, e := f.Service.Refresh(ctx, r.ID, id1); e != nil {
		t.Fatal(e)
	}
	rels, _ = f.Service.Relations(ctx, r.ID)
	if len(rels) != 1 || rels[0].AccessID == nil || *rels[0].AccessID != id2 {
		t.Fatal("complete observation erased other path facts")
	}
}
func TestConcurrentImportAtomicity(t *testing.T) {
	f := fxt.New(t)
	i := f.Instance(t, "public")
	c := f.Connection(t, i.ID, "write")
	b := f.Bind(t, c, "A", "live")
	f.Put("fixture.project", "repo", []string{"r1"})
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := f.Service.Import(ctx, b.ID, "fixture.project", fxt.Doc(map[string]any{"id": "repo"}))
			if e != nil {
				errs <- e
			} else {
				ids <- r.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if first != id {
			t.Fatal("dedup race")
		}
	}
	all, _ := f.Service.Resources(ctx)
	if len(all) != 1 {
		t.Fatal("duplicates")
	}
	f.Put("fixture.project", "broken", []string{"broken"}, catalog.ObservedRelation{Type: "undeclared", Reference: fxt.Reference(i.ID, "fixture.project", "repo", []string{"r1"})})
	if _, e := f.Service.Import(ctx, b.ID, "fixture.project", fxt.Doc(map[string]any{"id": "broken"})); e == nil {
		t.Fatal("invalid relation committed")
	}
	all, _ = f.Service.Resources(ctx)
	if len(all) != 1 {
		t.Fatal("resource+relation transaction was not atomic")
	}
}
func TestOperationDeletionProvenanceSecretsAndReplay(t *testing.T) {
	f := fxt.New(t)
	i := f.Instance(t, "public")
	c := f.Connection(t, i.ID, "write")
	b := f.Bind(t, c, "A", "live")
	input := catalog.OperationInput{TypeID: "fixture.project", ActionID: "create", TargetKind: "scope", TargetID: b.ID, Input: fxt.Doc(map[string]any{"id": "created", "password": "product-secret-123"})}
	op, e := f.Service.Submit(ctx, "create", input)
	if e != nil || op.Status != "succeeded" {
		t.Fatalf("create %v %v", op, e)
	}
	all, _ := f.Service.Resources(ctx)
	if len(all) != 1 || all[0].Origin != "mevius" {
		t.Fatal("creation evidence lost")
	}
	r := &all[0]
	if _, e = f.Service.SetProtection(ctx, r.ID, true); e != nil {
		t.Fatal(e)
	}
	if _, e = f.Service.Submit(ctx, "delete-protected", catalog.OperationInput{TypeID: r.TypeID, ActionID: "delete", TargetKind: "resource", TargetID: r.ID, Input: fxt.Doc(map[string]any{})}); !errors.Is(e, catalog.ErrForbidden) {
		t.Fatal(e)
	}
	r, e = f.Service.SetProtection(ctx, r.ID, false)
	if e != nil || r.Origin != "mevius" {
		t.Fatal("protection overwrote origin")
	}
	var count int
	for _, table := range []string{"resource_instance", "resource_access", "operation_request", "provider_connection", "connection_scope", "provider_scope"} {
		rows, e := f.Service.DB.QueryContext(ctx, "SELECT * FROM "+table)
		if e != nil {
			t.Fatal(e)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(cols))
			pointers := make([]any, len(cols))
			for j := range values {
				pointers[j] = &values[j]
			}
			if e = rows.Scan(pointers...); e != nil {
				t.Fatal(e)
			}
			for _, v := range values {
				if text, ok := v.(string); ok && (strings.Contains(text, "product-secret-123") || strings.Contains(text, "must-not-persist") || strings.Contains(text, "stripped")) {
					t.Fatal("secret persisted in " + table)
				}
			}
			count++
		}
		rows.Close()
	}
	if count == 0 {
		t.Fatal("empty secret scan")
	}
	del := f.Op(t, r, "", "delete", "delete")
	if del.Status != "succeeded" {
		t.Fatal(del)
	}
	if _, e = f.Service.Resource(ctx, r.ID); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("remote delete retained local resource")
	}
	got, e := f.Service.Operation(ctx, op.ID)
	if e != nil || got.Status != "succeeded" {
		t.Fatal("history depended on resource")
	}
	replayed, e := f.Service.Submit(ctx, "create", input)
	if e != nil || replayed.ID != op.ID || f.Calls["create"] != 1 {
		t.Fatal("replayed remote write")
	}
	reimport := f.Import(t, b, "fixture.project", "created")
	if reimport.Origin != "external" {
		t.Fatal("forget/reimport restored delete qualification")
	}
	if _, e = f.Service.Submit(ctx, "delete-import", catalog.OperationInput{TypeID: reimport.TypeID, ActionID: "delete", TargetKind: "resource", TargetID: reimport.ID, Input: fxt.Doc(map[string]any{})}); !errors.Is(e, catalog.ErrForbidden) {
		t.Fatal("import remotely deletable", e)
	}
}
func TestUnknownNoBlindRetryAndRunningConflict(t *testing.T) {
	f := fxt.New(t)
	i := f.Instance(t, "public")
	c := f.Connection(t, i.ID, "write")
	b := f.Bind(t, c, "A", "live")
	input := catalog.OperationInput{TypeID: "fixture.project", ActionID: "create", TargetKind: "scope", TargetID: b.ID, Input: fxt.Doc(map[string]any{"id": "unknown"})}
	f.Unknown = true
	op, e := f.Service.Submit(ctx, "unknown", input)
	if e != nil || op.Status != "unknown" {
		t.Fatal(op, e)
	}
	again, e := f.Service.Submit(ctx, "unknown", input)
	if e != nil || again.ID != op.ID || f.Calls["create"] != 1 {
		t.Fatal("unknown retried")
	}
	input.Input = fxt.Doc(map[string]any{"id": "different"})
	if _, e = f.Service.Submit(ctx, "unknown", input); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("key payload mismatch accepted")
	}
	f.Unknown = false
	f.Block = make(chan struct{})
	f.Entered = make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() { _, e := f.Service.Submit(ctx, "running", input); done <- e }()
	<-f.Entered
	if e = f.Service.DeleteConnection(ctx, c.ID); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("deleted running authorization", e)
	}
	if _, e = f.Service.RotateCredential(ctx, c.ID, []byte("new")); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("rotated running authorization", e)
	}
	close(f.Block)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
}

func TestNaturalIdentityRequiresExplicitMerge(t *testing.T) {
	f := fxt.New(t)
	i := f.Instance(t, "public")
	c := f.Connection(t, i.ID, "write")
	b := f.Bind(t, c, "A", "live")
	f.Put("fixture.webhook", "name", []string{"name"})
	r := f.Import(t, b, "fixture.webhook", "name")
	if _, e := f.Service.Import(ctx, b.ID, "fixture.webhook", fxt.Doc(map[string]any{"id": "name"})); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("natural identity was automatically merged", e)
	}
	if _, e := f.Service.ImportWithConfirmation(ctx, b.ID, "fixture.webhook", fxt.Doc(map[string]any{"id": "name"}), r.ID); e != nil {
		t.Fatal(e)
	}
}
func TestSchemaReadWriteVersionsAndUnknownHistoricalTypes(t *testing.T) {
	f := fxt.New(t)
	i := f.Instance(t, "public")
	c := f.Connection(t, i.ID, "write")
	b := f.Bind(t, c, "A", "live")
	f.Put("fixture.project", "good", []string{"good"})
	r := f.Import(t, b, "fixture.project", "good")
	bad := f.Observations["fixture.project:good"]
	bad.Public = fxt.Raw(map[string]any{"label": "good", "signing_secret": "leak"})
	f.Observations["fixture.project:good"] = bad
	if _, e := f.Service.Import(ctx, b.ID, "fixture.project", fxt.Doc(map[string]any{"id": "good"})); e == nil {
		t.Fatal("unknown secret observation accepted")
	}
	f.Service.DB.Exec(`UPDATE resource_access SET observation_version=2 WHERE resource_id=?`, r.ID)
	if _, e := f.Service.Accesses(ctx, r.ID); e == nil {
		t.Fatal("unknown observation version silently read")
	}
	f.Service.DB.Exec(`UPDATE resource_access SET observation_version=1 WHERE resource_id=?`, r.ID)
	f.Service.DB.Exec(`UPDATE resource_instance SET resource_type_id='historical.unknown' WHERE id=?`, r.ID)
	if _, e := f.Service.Resource(ctx, r.ID); e != nil {
		t.Fatal("unregistered historical identity inaccessible", e)
	}
	if _, e := f.Service.Accesses(ctx, r.ID); e != nil {
		t.Fatal("historical public snapshot inaccessible", e)
	}
	if _, _, e := f.Service.SelectAccess(ctx, &catalog.Resource{ID: r.ID, TypeID: "historical.unknown"}, ""); !errors.Is(e, catalog.ErrForbidden) {
		t.Fatal("historical unregistered type executable")
	}
}
func TestAdditionalProviderDoesNotRequireCoreBranches(t *testing.T) {
	f := fxt.New(t)
	d := f.Provider.Descriptors[0].ResourceTypes[0]
	d.ID = "saas.webhook"
	d.ProductID = "saas.integration"
	d.ProviderID = "saas"
	d.Category = "integration"
	d.Roles = []string{"integration_owner"}
	p := &fxt.Provider{ProviderID: "saas", Descriptors: []catalog.ProductDescriptor{{ID: d.ProductID, ProviderID: "saas", Name: "SaaS integration", ResourceTypes: []catalog.ResourceTypeDescriptor{d}}}}
	config := catalog.ObjectSchema(map[string]any{"api_url": fxt.Text(), "events_url": fxt.Text()}, "api_url", "events_url")
	p.Configuration = &config
	f.Service.Registry.RegisterProvider(p)
	handler := &fxt.Handler{F: f, Type: d}
	actions := map[string]catalog.ActionHandler{}
	for _, action := range d.Actions {
		actions[action.ID] = func(context.Context, catalog.AccessContext, *catalog.Resource, *catalog.Access, catalog.JSON) (catalog.ActionResult, error) {
			return catalog.ActionResult{Public: fxt.Raw(map[string]any{})}, nil
		}
	}
	f.Service.Registry.RegisterHandler(d.ID, handler, map[string]catalog.ViewHandler{}, actions)
	if e := f.Service.Registry.ValidateReady(); e != nil {
		t.Fatal(e)
	}
	i, e := f.Service.CreateInstance(ctx, "saas", "cloud", fxt.Doc(map[string]any{"api_url": "https://saas.example/api", "events_url": "https://events.example/api"}))
	if e != nil {
		t.Fatal(e)
	}
	c := f.Connection(t, i.ID, "write")
	b := f.Bind(t, c, "A", "sandbox")
	f.Put(d.ID, "hook", []string{"hook-id"})
	r := f.Import(t, b, d.ID, "hook")
	if r.TypeID != "saas.webhook" {
		t.Fatal(r)
	}
}

func TestDatabaseParentNamespaceAndLateResolution(t *testing.T) {
	f := fxt.New(t)
	i := f.Instance(t, "public")
	c := f.Connection(t, i.ID, "write")
	b := f.Bind(t, c, "A", "live")
	parent := catalog.ObservedRelation{Type: "parent", Reference: fxt.Reference(i.ID, "fixture.branch", "branch", []string{"p", "b1"})}
	f.Put("fixture.database", "db", []string{"p", "b1", "db"}, parent)
	first := f.Import(t, b, "fixture.database", "db")
	parent.Reference = fxt.Reference(i.ID, "fixture.branch", "other-branch", []string{"p", "b2"})
	f.Put("fixture.database", "db", []string{"p", "b2", "db"}, parent)
	second := f.Import(t, b, "fixture.database", "db")
	if first.ID == second.ID {
		t.Fatal("same local database ID collided across parents")
	}
	f.Put("fixture.branch", "branch", []string{"p", "b1"}, catalog.ObservedRelation{Type: "parent", Reference: fxt.Reference(i.ID, "fixture.project", "project", []string{"p"})})
	branch := f.Import(t, b, "fixture.branch", "branch")
	rels, e := f.Service.Relations(ctx, first.ID)
	if e != nil || rels[0].Reference.ResolvedID == nil || *rels[0].Reference.ResolvedID != branch.ID {
		t.Fatal("late parent not resolved", e)
	}
	f.Put("fixture.project", "project", []string{"p"})
	f.Import(t, b, "fixture.project", "project")
}
func TestLocatorOnlyReferenceNeedsRemoteProof(t *testing.T) {
	f := fxt.New(t)
	i := f.Instance(t, "public")
	c := f.Connection(t, i.ID, "write")
	b := f.Bind(t, c, "A", "live")
	f.Put("fixture.project", "repo", []string{"verified-repo"})
	target := f.Import(t, b, "fixture.project", "repo")
	f.Put("fixture.webhook", "hook", []string{"hook"}, catalog.ObservedRelation{Type: "source_repo", Reference: fxt.Reference(i.ID, "fixture.project", "repo", nil)})
	from := f.Import(t, b, "fixture.webhook", "hook")
	relations, e := f.Service.Relations(ctx, from.ID)
	if e != nil || relations[0].Reference.ResolvedID != nil {
		t.Fatal("guessed locator resolution")
	}
	resolved, e := f.Service.ResolveReference(ctx, relations[0].Reference.ID, b.ID)
	if e != nil || resolved.ID != target.ID {
		t.Fatal("proof failed", e)
	}
	relations, e = f.Service.Relations(ctx, from.ID)
	if e != nil || relations[0].Reference.ResolvedID == nil || *relations[0].Reference.ResolvedID != target.ID {
		t.Fatal("reference stayed unresolved")
	}
}
func TestPreflightRejectsSameNameRecreation(t *testing.T) {
	f := fxt.New(t)
	i := f.Instance(t, "public")
	c := f.Connection(t, i.ID, "write")
	b := f.Bind(t, c, "A", "live")
	input := catalog.OperationInput{TypeID: "fixture.project", ActionID: "create", TargetKind: "scope", TargetID: b.ID, Input: fxt.Doc(map[string]any{"id": "created"})}
	if _, e := f.Service.Submit(ctx, "create", input); e != nil {
		t.Fatal(e)
	}
	all, _ := f.Service.Resources(ctx)
	r := all[0]
	f.Put("fixture.project", "created", []string{"recreated-remote-uid"})
	op, e := f.Service.Submit(ctx, "delete-recreated", catalog.OperationInput{TypeID: r.TypeID, ActionID: "delete", TargetKind: "resource", TargetID: r.ID, Input: fxt.Doc(map[string]any{})})
	if e != nil || op.Status != "failed" || op.ErrorCode != "identity_changed" || f.Calls["delete"] != 0 {
		t.Fatal("same-name recreation deleted", op, e)
	}
}

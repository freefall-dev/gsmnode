// Package bootstrap brings a fresh PocketBase up to the schema the API Server
// expects, on server startup. It is the Go port of scripts/setup-pocketbase.mjs
// and is idempotent: existing collections are reconciled (missing fields added,
// relation/select options fixed) rather than recreated, and an already-present
// super-admin user is left untouched.
//
// It creates the app-level collections and, optionally, the gsmnode super-admin
// account. It does NOT create the PocketBase *superuser* — that is a
// chicken-and-egg the REST API can't solve (creating a _superusers record needs
// an existing superuser token), so the PocketBase container upserts it from env
// on boot instead. Bootstrap authenticates with that same superuser.
package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"smsgateway/apiserver/internal/pb"
)

// Options configures a bootstrap run. SuperAdmin* are optional: when either the
// email or password is empty, the super-admin step is skipped.
type Options struct {
	UsersCollection    string
	SuperAdminEmail    string
	SuperAdminPassword string
	SuperAdminName     string
}

// fieldDef is a schema field normalized to a single shape; it is rendered into
// the right PocketBase wire format ("fields" for v0.23+, legacy "schema") per
// the detected server version.
type fieldDef struct {
	name          string
	typ           string
	required      bool
	relTo         string // relation target collection name
	cascadeDelete bool   // relations only; default true
	values        []string
	onCreate      bool // autodate only
	onUpdate      bool // autodate only
	maxSize       int  // json only, bytes
}

// Field builders, mirroring the f.* helpers in setup-pocketbase.mjs.
func fText(name string, required bool) fieldDef {
	return fieldDef{name: name, typ: "text", required: required}
}
func fNumber(name string) fieldDef { return fieldDef{name: name, typ: "number"} }
func fDate(name string, required bool) fieldDef {
	return fieldDef{name: name, typ: "date", required: required}
}
func fURL(name string, required bool) fieldDef {
	return fieldDef{name: name, typ: "url", required: required}
}
func fRelation(name, relTo string, required, cascade bool) fieldDef {
	return fieldDef{name: name, typ: "relation", required: required, relTo: relTo, cascadeDelete: cascade}
}
func fSelect(name string, values []string, required bool) fieldDef {
	return fieldDef{name: name, typ: "select", required: required, values: values}
}
func fAutodate(name string, onCreate, onUpdate bool) fieldDef {
	return fieldDef{name: name, typ: "autodate", onCreate: onCreate, onUpdate: onUpdate}
}
func fJSON(name string, required bool, maxSize int) fieldDef {
	return fieldDef{name: name, typ: "json", required: required, maxSize: maxSize}
}

// renderField turns a fieldDef into the map PocketBase expects, in either the
// modern ("fields") or legacy ("schema") layout. idByName resolves a relation's
// target collection name to its id.
func renderField(d fieldDef, format string, idByName map[string]string) map[string]any {
	if format == "schema" {
		options := map[string]any{}
		switch d.typ {
		case "relation":
			options["collectionId"] = idByName[d.relTo]
			options["cascadeDelete"] = d.cascadeDelete
			options["maxSelect"] = 1
			options["minSelect"] = 0
		case "select":
			options["values"] = d.values
			options["maxSelect"] = 1
		case "json":
			options["maxSize"] = d.maxSize
		}
		return map[string]any{"name": d.name, "type": d.typ, "required": d.required, "options": options}
	}

	field := map[string]any{"name": d.name, "type": d.typ, "required": d.required}
	switch d.typ {
	case "relation":
		field["collectionId"] = idByName[d.relTo]
		field["cascadeDelete"] = d.cascadeDelete
		field["maxSelect"] = 1
		field["minSelect"] = 0
	case "select":
		field["values"] = d.values
		field["maxSelect"] = 1
	case "autodate":
		field["onCreate"] = d.onCreate
		field["onUpdate"] = d.onUpdate
	case "json":
		field["maxSize"] = d.maxSize
	}
	return field
}

// Run authenticates as the superuser, creates any missing collections, reconciles
// existing ones, and (when configured) ensures the gsmnode super-admin exists.
// It is safe to call on every startup.
func Run(ctx context.Context, client *pb.Client, opts Options) error {
	if err := client.Authenticate(ctx); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	existing, err := listCollections(ctx, client)
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	format := detectFormat(existing)
	log.Printf("bootstrap: schema format %q", format)

	present := make(map[string]bool, len(existing))
	idByName := make(map[string]string, len(existing))
	for _, c := range existing {
		present[c.Name] = true
		idByName[c.Name] = c.ID
	}

	// Create in dependency order: organizations before users references it;
	// devices before messages/inbox/webhooks reference it. "users" is
	// PocketBase's built-in auth collection and is never created here — only
	// reconciled below.
	for _, name := range createOrder {
		if present[name] {
			continue
		}
		if err := createCollection(ctx, client, name, format, idByName); err != nil {
			return fmt.Errorf("create %s: %w", name, err)
		}
		log.Printf("bootstrap: %s created", name)
		// Refresh so later relations resolve newly-created collection ids.
		refreshed, err := listCollections(ctx, client)
		if err != nil {
			return fmt.Errorf("re-list collections: %w", err)
		}
		for _, c := range refreshed {
			present[c.Name] = true
			idByName[c.Name] = c.ID
		}
	}

	// Reconcile fields on existing collections (add missing + fix relation
	// cascade and select value lists).
	for _, name := range reconcileOrder {
		if err := reconcileFields(ctx, client, name, format, idByName); err != nil {
			return fmt.Errorf("reconcile %s: %w", name, err)
		}
	}

	if err := ensureSuperAdmin(ctx, client, opts); err != nil {
		return fmt.Errorf("super-admin: %w", err)
	}
	return nil
}

// --- PocketBase collection calls -------------------------------------------

type collectionMeta struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Fields []map[string]any `json:"fields"`
	Schema []map[string]any `json:"schema"`
}

func listCollections(ctx context.Context, client *pb.Client) ([]collectionMeta, error) {
	raw, status, err := client.Raw(ctx, http.MethodGet, "/api/collections?perPage=200", nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("status %d: %s", status, raw)
	}
	var env struct {
		Items []collectionMeta `json:"items"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	return env.Items, nil
}

// detectFormat reports whether this PocketBase serializes fields under "fields"
// (v0.23+) or the legacy "schema" key, defaulting to the modern format.
func detectFormat(cols []collectionMeta) string {
	for _, c := range cols {
		if len(c.Fields) > 0 {
			return "fields"
		}
		if len(c.Schema) > 0 {
			return "schema"
		}
	}
	return "fields"
}

func createCollection(ctx context.Context, client *pb.Client, name, format string, idByName map[string]string) error {
	rendered := make([]map[string]any, 0, len(collectionsSchema[name]))
	for _, d := range collectionsSchema[name] {
		rendered = append(rendered, renderField(d, format, idByName))
	}
	body := map[string]any{
		"name": name,
		"type": "base",
		format: rendered, // "fields" or "schema"
		// Rules left null => superuser-only access (the API Server is the only client).
		"listRule":   nil,
		"viewRule":   nil,
		"createRule": nil,
		"updateRule": nil,
		"deleteRule": nil,
	}
	if idx := indexes[name]; len(idx) > 0 {
		body["indexes"] = idx
	}
	raw, status, err := client.Raw(ctx, http.MethodPost, "/api/collections", body)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("status %d: %s", status, raw)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &created); err == nil {
		idByName[name] = created.ID
	}
	return nil
}

// reconcileFields brings an existing collection in line with the desired schema:
// it appends missing fields and updates relation cascadeDelete and select value
// lists on existing fields, preserving every existing field (including system
// fields) and their ids.
func reconcileFields(ctx context.Context, client *pb.Client, name, format string, idByName map[string]string) error {
	raw, status, err := client.Raw(ctx, http.MethodGet, "/api/collections/"+name, nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("get: status %d: %s", status, raw)
	}
	var col collectionMeta
	if err := json.Unmarshal(raw, &col); err != nil {
		return err
	}
	current := col.Fields
	if format == "schema" {
		current = col.Schema
	}

	defs := collectionsSchema[name]
	defByName := make(map[string]fieldDef, len(defs))
	for _, d := range defs {
		defByName[d.name] = d
	}

	var changes []string
	merged := make([]map[string]any, 0, len(current)+len(defs))
	haveName := make(map[string]bool, len(current))
	for _, f := range current {
		fname, _ := f["name"].(string)
		haveName[fname] = true
		if def, ok := defByName[fname]; ok {
			switch def.typ {
			case "relation":
				if asBool(f["cascadeDelete"]) != def.cascadeDelete {
					f["cascadeDelete"] = def.cascadeDelete
					changes = append(changes, fname+".cascadeDelete")
				}
			case "select":
				if !sameValues(f["values"], def.values) {
					f["values"] = def.values
					changes = append(changes, fname+".values")
				}
			}
		}
		merged = append(merged, f)
	}
	// Append missing fields.
	for _, d := range defs {
		if !haveName[d.name] {
			merged = append(merged, renderField(d, format, idByName))
			changes = append(changes, "+"+d.name)
		}
	}

	if len(changes) == 0 {
		log.Printf("bootstrap: %s up to date", name)
		return nil
	}
	patch := map[string]any{format: merged}
	raw, status, err = client.Raw(ctx, http.MethodPatch, "/api/collections/"+col.ID, patch)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("patch: status %d: %s", status, raw)
	}
	log.Printf("bootstrap: %s — %v", name, changes)
	return nil
}

// ensureSuperAdmin creates the gsmnode super-admin user when it does not yet
// exist. An already-present account (matched by email) is left untouched.
func ensureSuperAdmin(ctx context.Context, client *pb.Client, opts Options) error {
	if opts.SuperAdminEmail == "" || opts.SuperAdminPassword == "" {
		return nil // not requested
	}
	coll := opts.UsersCollection
	if coll == "" {
		coll = "users"
	}

	filter := url.QueryEscape(fmt.Sprintf("email='%s'", opts.SuperAdminEmail))
	raw, status, err := client.Raw(ctx, http.MethodGet,
		"/api/collections/"+coll+"/records?perPage=1&filter="+filter, nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("lookup: status %d: %s", status, raw)
	}
	var found struct {
		TotalItems int `json:"totalItems"`
	}
	if err := json.Unmarshal(raw, &found); err != nil {
		return err
	}
	if found.TotalItems > 0 {
		log.Printf("bootstrap: super-admin %s already exists", opts.SuperAdminEmail)
		return nil
	}

	name := opts.SuperAdminName
	if name == "" {
		name = "Administrator"
	}
	create := map[string]any{
		"email":           opts.SuperAdminEmail,
		"password":        opts.SuperAdminPassword,
		"passwordConfirm": opts.SuperAdminPassword,
		"name":            name,
		"role":            "superadmin",
		"verified":        true,
		"emailVisibility": false,
	}
	raw, status, err = client.Raw(ctx, http.MethodPost, "/api/collections/"+coll+"/records", create)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("create: status %d: %s", status, raw)
	}
	log.Printf("bootstrap: super-admin %s created", opts.SuperAdminEmail)
	return nil
}

// --- helpers ---------------------------------------------------------------

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

// sameValues reports whether a select field's current values equal the desired
// set (order-insensitive), matching the reconcile check in setup-pocketbase.mjs.
func sameValues(current any, want []string) bool {
	arr, ok := current.([]any)
	if !ok || len(arr) != len(want) {
		return false
	}
	have := make(map[string]bool, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			have[s] = true
		}
	}
	for _, w := range want {
		if !have[w] {
			return false
		}
	}
	return true
}

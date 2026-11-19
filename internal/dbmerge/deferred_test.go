package dbmerge

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func TestBuildProviderRejectsDeferredStaticBaseBeforeCredentials(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, caller := range []bool{false, true} {
			for _, empty := range []bool{false, true} {
				bases := testBases()
				base := bases["base"]
				if caller {
					base.Type = config.ProviderTypeOpenAICompatible
					d := config.DefaultDeferredTransport()
					if empty {
						d = config.DeferredTransport{}
					}
					base.DeferredTransport = &d
				} else {
					d := config.DeferredReceiver{ExecutionTimeout: time.Minute, MaxResultBytes: 4096}
					if empty {
						d = config.DeferredReceiver{}
					}
					base.DeferredReceiver = &d
				}
				bases["base"] = base
				row := store.DBProvider{
					Name: "managed", Type: string(base.Type), Extends: base.Name, Enabled: enabled,
					APIKeyEncrypted: []byte("not-a-valid-encrypted-credential"),
				}
				before := config.NewCatalog([]config.Provider{base}, nil, nil).Providers()
				_, err := BuildProvider(row, nil, bases)
				if err == nil || !strings.Contains(err.Error(), "static-config-only") || strings.Contains(err.Error(), "decrypt") {
					t.Fatalf("managed opted-base inheritance reached credential handling: %v", err)
				}
				after := config.NewCatalog([]config.Provider{bases["base"]}, nil, nil).Providers()
				if !reflect.DeepEqual(before, after) {
					t.Fatal("rejected managed derivative mutated the static base")
				}
			}
		}
	}
	bases := testBases()
	p, err := BuildProvider(store.DBProvider{Name: "managed", Type: "openai", Extends: "base", Enabled: false}, nil, bases)
	if err != nil || p.DeferredTransport != nil || p.DeferredReceiver != nil {
		t.Fatalf("ordinary managed inheritance changed: %v", err)
	}
}

func TestMergeCatalogDeferredBaseRejectionPreservesStaticPolicy(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, testDBURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	base := testBases()["base"]
	base.Name = fmt.Sprintf("deferred-base-%d", time.Now().UnixNano())
	row := store.DBProvider{Name: fmt.Sprintf("deferred-derived-%d", time.Now().UnixNano()), Type: "openai", Extends: base.Name, Enabled: false}
	row = corruptReasoningProvider(t, st, row, nil)
	t.Cleanup(func() { _ = st.DeleteProvider(ctx, row.ID) })
	for _, caller := range []bool{false, true} {
		base.DeferredTransport, base.DeferredReceiver = nil, nil
		if caller {
			d := config.DefaultDeferredTransport()
			base.DeferredTransport = &d
		} else {
			base.DeferredReceiver = &config.DeferredReceiver{ExecutionTimeout: time.Minute, MaxResultBytes: 4096}
		}
		rt := &config.Runtime{Deferred: config.DefaultDeferred(), Catalog: config.NewCatalog([]config.Provider{base}, nil, nil)}
		before := rt.Catalog.Providers()
		if _, err := MergeCatalog(ctx, st, rt); err == nil || !strings.Contains(err.Error(), "static-config-only") {
			t.Fatalf("stored derivative of opted base accepted: %v", err)
		}
		if !reflect.DeepEqual(before, rt.Catalog.Providers()) {
			t.Fatal("failed merge changed static deferred policy")
		}
	}
}

func deferredModelMergeBase(view string, empty bool) config.Provider {
	base := testBases()["base"]
	base.Type = config.ProviderTypeOpenAICompatible
	model := base.Models[0]
	options := config.DefaultDeferredTransport()
	if empty {
		options = config.DeferredTransport{}
	}
	opted := model
	opted.DeferredTransport = &options
	switch view {
	case "ordered":
		base.Models[0] = opted
		base.ModelByName = nil
	case "indexed":
		base.Models = nil
		base.ModelByName[model.Name] = opted
	case "ordered-with-nil-index":
		base.Models[0] = opted
	case "indexed-with-nil-ordered":
		base.ModelByName[model.Name] = opted
	case "index-only":
		base.Models = nil
		base.ModelByName = map[string]config.Model{"index-only": {DeferredTransport: &options}}
	case "conflicting":
		base.Models[0] = opted
		other := config.DeferredTransport{}
		model.DeferredTransport = &other
		base.ModelByName[model.Name] = model
	case "indexed-extra":
		base.ModelByName["extra"] = opted
	}
	return base
}

func TestBuildProviderRejectsDeferredModelStaticBaseBeforeCredentials(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, empty := range []bool{false, true} {
			for _, view := range []string{"ordered", "indexed", "ordered-with-nil-index", "indexed-with-nil-ordered", "index-only", "conflicting", "indexed-extra"} {
				t.Run(view+"/"+map[bool]string{false: "disabled", true: "enabled"}[enabled]+"/"+map[bool]string{false: "configured", true: "empty"}[empty], func(t *testing.T) {
					base := deferredModelMergeBase(view, empty)
					bases := map[string]config.Provider{base.Name: base}
					before := config.NewCatalog([]config.Provider{base}, nil, nil).Providers()
					row := store.DBProvider{Name: "managed", Type: string(base.Type), Extends: base.Name, Enabled: enabled, APIKeyEncrypted: []byte("invalid-encrypted-credential")}
					if _, err := BuildProvider(row, nil, bases); err == nil || !strings.Contains(err.Error(), "static-config-only") || strings.Contains(err.Error(), "decrypt") {
						t.Fatalf("managed model-opted inheritance reached credentials: %v", err)
					}
					if !reflect.DeepEqual(before, config.NewCatalog([]config.Provider{bases[base.Name]}, nil, nil).Providers()) {
						t.Fatal("rejected managed model inheritance mutated static policy")
					}
				})
			}
		}
	}
	base := deferredModelMergeBase("ordinary", false)
	p, err := BuildProvider(store.DBProvider{Name: "managed", Type: string(base.Type), Extends: base.Name, Enabled: false}, nil, map[string]config.Provider{base.Name: base})
	if err != nil || config.HasCallerDeferredTransport(p) || p.DeferredReceiver != nil {
		t.Fatalf("ordinary managed inheritance changed: %v", err)
	}
}

func TestMergeCatalogDeferredModelBaseRejectionPreservesStaticPolicy(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, testDBURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	baseName := fmt.Sprintf("deferred-model-base-%d", time.Now().UnixNano())
	row := store.DBProvider{Name: fmt.Sprintf("deferred-model-derived-%d", time.Now().UnixNano()), Type: "openai-compatible", Extends: baseName, Enabled: false}
	row = corruptReasoningProvider(t, st, row, nil)
	t.Cleanup(func() { _ = st.DeleteProvider(ctx, row.ID) })
	for _, view := range []string{"ordered", "indexed-with-nil-ordered", "index-only", "conflicting"} {
		for _, empty := range []bool{false, true} {
			base := deferredModelMergeBase(view, empty)
			base.Name = baseName
			rt := &config.Runtime{Catalog: config.NewCatalog([]config.Provider{base}, nil, nil)}
			before := rt.Catalog.Providers()
			if _, err := MergeCatalog(ctx, st, rt); err == nil || !strings.Contains(err.Error(), "static-config-only") {
				t.Fatalf("stored derivative of model-opted base accepted: %v", err)
			}
			if !reflect.DeepEqual(before, rt.Catalog.Providers()) {
				t.Fatal("failed model-opted merge changed static policy")
			}
		}
	}
}

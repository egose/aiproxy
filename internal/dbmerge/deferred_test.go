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
	if err := st.CreateProviderAggregate(ctx, &row, nil); err != nil {
		t.Fatal(err)
	}
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

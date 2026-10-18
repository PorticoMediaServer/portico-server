package httpapi

import (
	"net/url"
	"testing"

	"portico.local/server/internal/catalog"
)

func TestSavedViewPresetsPinOrderInvalidatesContinuation(t *testing.T) {
	f := newV1Fixture(t, 0)
	device := f.device("preset-order-other-device")
	var order []catalog.PinOrderEntry
	for _, name := range []string{"Zulu", "Alpha", "Charlie"} {
		r := presetHTTPCreate(t, f, f.owner.AccessToken, "pin-order-create-"+name, name, presetHTTPDefinition(f.library, "movies"))
		f.call("PUT", "/v1/saved-pins/view/"+r.ResourceID, nil, map[string]any{"operationId": "pin-order-pin-" + name, "expectedRevision": 0, "pinned": true}, 200, nil)
		order = append(order, catalog.PinOrderEntry{Kind: "view", ID: r.ResourceID})
	}
	var arranged catalog.PinOrder
	f.call("PUT", "/v1/saved-pins/order", nil, map[string]any{"expectedRevision": 0, "order": order}, 200, &arranged)
	query := "kind=view&libraryId=" + f.library + "&pivot=movies&pinned=true&limit=1"
	page := presetHTTPPage(f, f.owner.AccessToken, query)
	presetHTTPIDs(t, page, order[0].ID)
	if page.NextCursor == "" {
		t.Fatal("missing continuation")
	}
	order[0], order[1] = order[1], order[0]
	f.callAs(device.AccessToken, "PUT", "/v1/saved-pins/order", nil, map[string]any{"expectedRevision": arranged.Revision, "order": order}, 200, nil)
	w := f.raw("GET", "/v1/saved-resources?"+query+"&cursor="+url.QueryEscape(page.NextCursor), f.owner.AccessToken, nil, nil)
	if w.Code != 409 || v1Code(w) != "stale_continuation" {
		t.Fatalf("reordered cursor: %d %s", w.Code, w.Body.String())
	}
	fresh := presetHTTPPage(f, device.AccessToken, query)
	presetHTTPIDs(t, fresh, order[0].ID)
	next := presetHTTPPage(f, f.owner.AccessToken, query+"&cursor="+url.QueryEscape(fresh.NextCursor))
	presetHTTPIDs(t, next, order[1].ID)
}

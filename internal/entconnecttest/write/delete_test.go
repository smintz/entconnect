package write

import (
	"context"
	"strconv"
	"testing"

	"connectrpc.com/connect"

	"github.com/smintz/entconnect/internal/entconnecttest/write/ent/schema"
	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
)

// TestDeleteItem_RealRoundTrip is the write slice's architectural proof
// for Delete: a real Connect client removes a real row through a
// generated handler, inside the fixed interceptor chain, against a real
// ent client this test never touches directly except to seed/verify.
func TestDeleteItem_RealRoundTrip(t *testing.T) {
	client := newTestClient(t)
	authn := testAuthenticator{subject: "allowed-subject"}
	createClient, deleteClient := newTestServer(t, client, authn)

	created, err := createClient.CreateItem(context.Background(), connect.NewRequest(&entconnecttestv1.CreateItemRequest{
		Item: &entconnecttestv1.Item{Sku: "SKU-DELETE", Name: "Widget", Quantity: 5},
	}))
	if err != nil {
		t.Fatalf("seed CreateItem: %v", err)
	}

	_, err = deleteClient.DeleteItem(context.Background(), connect.NewRequest(&entconnecttestv1.DeleteItemRequest{
		Id: created.Msg.GetItem().GetId(),
	}))
	if err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}

	// Assert the row is actually gone from the ent client.
	id, err := strconv.Atoi(created.Msg.GetItem().GetId())
	if err != nil {
		t.Fatalf("parse created id: %v", err)
	}
	if _, err := client.Item.Get(context.Background(), id); err == nil {
		t.Fatal("want the row to be gone after DeleteItem, but it is still queryable")
	}
}

// TestDeleteItem_RepeatIsNotFound proves CRUD-02's idempotency edge:
// deleting the same identifier a second time returns CodeNotFound.
func TestDeleteItem_RepeatIsNotFound(t *testing.T) {
	client := newTestClient(t)
	authn := testAuthenticator{subject: "allowed-subject"}
	createClient, deleteClient := newTestServer(t, client, authn)

	created, err := createClient.CreateItem(context.Background(), connect.NewRequest(&entconnecttestv1.CreateItemRequest{
		Item: &entconnecttestv1.Item{Sku: "SKU-REPEAT-DELETE", Name: "Widget", Quantity: 1},
	}))
	if err != nil {
		t.Fatalf("seed CreateItem: %v", err)
	}
	req := connect.NewRequest(&entconnecttestv1.DeleteItemRequest{Id: created.Msg.GetItem().GetId()})

	if _, err := deleteClient.DeleteItem(context.Background(), req); err != nil {
		t.Fatalf("first DeleteItem: %v", err)
	}

	_, err = deleteClient.DeleteItem(context.Background(), req)
	if err == nil {
		t.Fatal("want an error for a repeat delete, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeNotFound {
		t.Fatalf("want CodeNotFound, got %v (%v)", code, err)
	}
}

// TestDeleteItem_UnrelatedRowSurvives is the concrete proof that the
// generated delete is scoped to the identified key only (T-02-08): it
// never touches any other row.
func TestDeleteItem_UnrelatedRowSurvives(t *testing.T) {
	client := newTestClient(t)
	authn := testAuthenticator{subject: "allowed-subject"}
	createClient, deleteClient := newTestServer(t, client, authn)

	first, err := createClient.CreateItem(context.Background(), connect.NewRequest(&entconnecttestv1.CreateItemRequest{
		Item: &entconnecttestv1.Item{Sku: "SKU-A", Name: "First", Quantity: 1},
	}))
	if err != nil {
		t.Fatalf("seed first CreateItem: %v", err)
	}
	second, err := createClient.CreateItem(context.Background(), connect.NewRequest(&entconnecttestv1.CreateItemRequest{
		Item: &entconnecttestv1.Item{Sku: "SKU-B", Name: "Second", Quantity: 2},
	}))
	if err != nil {
		t.Fatalf("seed second CreateItem: %v", err)
	}

	_, err = deleteClient.DeleteItem(context.Background(), connect.NewRequest(&entconnecttestv1.DeleteItemRequest{
		Id: first.Msg.GetItem().GetId(),
	}))
	if err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}

	secondID, err := strconv.Atoi(second.Msg.GetItem().GetId())
	if err != nil {
		t.Fatalf("parse second id: %v", err)
	}
	row, err := client.Item.Get(context.Background(), secondID)
	if err != nil {
		t.Fatalf("want the unrelated row to survive, got: %v", err)
	}
	if row.Sku != "SKU-B" {
		t.Fatalf("want surviving row sku %q, got %q", "SKU-B", row.Sku)
	}
}

// TestDeleteItem_PrivacyDenyIsPermissionDenied proves T-02-09: a real ent
// privacy.Deny decision on a mutation (schema.Item.Policy's DeniedSubject
// rule) arrives at the Connect client as CodePermissionDenied.
func TestDeleteItem_PrivacyDenyIsPermissionDenied(t *testing.T) {
	client := newTestClient(t)

	seedAuthn := testAuthenticator{subject: "allowed-subject"}
	createClient, _ := newTestServer(t, client, seedAuthn)
	created, err := createClient.CreateItem(context.Background(), connect.NewRequest(&entconnecttestv1.CreateItemRequest{
		Item: &entconnecttestv1.Item{Sku: "SKU-DENIED-DELETE", Name: "Widget", Quantity: 1},
	}))
	if err != nil {
		t.Fatalf("seed CreateItem: %v", err)
	}

	deniedAuthn := testAuthenticator{subject: schema.DeniedSubject}
	_, deleteClient := newTestServer(t, client, deniedAuthn)

	_, err = deleteClient.DeleteItem(context.Background(), connect.NewRequest(&entconnecttestv1.DeleteItemRequest{
		Id: created.Msg.GetItem().GetId(),
	}))
	if err == nil {
		t.Fatal("want an error for a denied viewer, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodePermissionDenied {
		t.Fatalf("want CodePermissionDenied, got %v (%v)", code, err)
	}

	// The row must still exist — the denied delete must not have run.
	id, err := strconv.Atoi(created.Msg.GetItem().GetId())
	if err != nil {
		t.Fatalf("parse created id: %v", err)
	}
	if _, err := client.Item.Get(context.Background(), id); err != nil {
		t.Fatalf("want the row to still exist after a denied delete, got: %v", err)
	}
}

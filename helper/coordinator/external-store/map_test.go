package externalstore

import "testing"

func TestResolveRoute_DefaultsExternalKeyToRef(t *testing.T) {
	c := &mapCoordinator[int]{
		routeForEntry: func(ctx RouteContext[int]) (Route, error) {
			return Route{
				UseExternal: true,
				Ref:         "cred-default",
			}, nil
		},
	}

	route, err := c.resolveRoute(RouteContext[int]{Key: "default"})
	if err != nil {
		t.Fatalf("resolveRoute() error = %v", err)
	}
	if route.ExternalKey != "cred-default" {
		t.Fatalf("route.ExternalKey = %q, want %q", route.ExternalKey, "cred-default")
	}
}

func TestResolveRoute_DefaultsExternalKeyForExistingRef(t *testing.T) {
	c := &mapCoordinator[int]{
		routeForEntry: func(ctx RouteContext[int]) (Route, error) {
			return Route{Ref: ctx.ExistingRef}, nil
		},
	}

	route, err := c.resolveRoute(RouteContext[int]{
		Key:         "default",
		ExistingRef: "cred-default",
	})
	if err != nil {
		t.Fatalf("resolveRoute() error = %v", err)
	}
	if route.ExternalKey != "cred-default" {
		t.Fatalf("route.ExternalKey = %q, want %q", route.ExternalKey, "cred-default")
	}
}

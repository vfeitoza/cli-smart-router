package main

import "testing"

func TestPluginRegistrationExposesRoutes(t *testing.T) {
	for _, field := range pluginRegistration().Metadata.ConfigFields {
		if field.Name == "routes" {
			return
		}
	}
	t.Fatal("expected routes in plugin configuration fields")
}

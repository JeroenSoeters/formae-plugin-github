// Package resources imports all resource provisioner packages to trigger
// their init() registration.
package resources

import (
	_ "github.com/platform-engineering-labs/formae-plugin-github/pkg/resources/actions"
	_ "github.com/platform-engineering-labs/formae-plugin-github/pkg/resources/repos"
	_ "github.com/platform-engineering-labs/formae-plugin-github/pkg/resources/teams"
)

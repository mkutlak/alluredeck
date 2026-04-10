package handlers

import "strings"

const namespaceSep = "--"

// NamespacedProjectID returns parentID--shortID when parentID is non-empty,
// allowing child projects with identical short names under different parents.
func NamespacedProjectID(parentID, shortID string) string {
	if parentID == "" {
		return shortID
	}
	return parentID + namespaceSep + shortID
}

// ShortProjectName extracts the display name from a namespaced project ID.
// For "roger-api-tests--api-licences" it returns "api-licences".
// For non-namespaced IDs it returns the ID unchanged.
func ShortProjectName(projectID string) string {
	if _, after, ok := strings.Cut(projectID, namespaceSep); ok {
		return after
	}
	return projectID
}

package mcp

import (
	"context"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Probe final ResourceContents usage.
var _ = func() {
	s := mcpsdk.NewServer(nil, nil)

	s.AddResource(&mcpsdk.Resource{
		URI:  "alluredeck://test/1",
		Name: "test",
	}, func(_ context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		textRC := &mcpsdk.ResourceContents{
			URI:      "alluredeck://test/1",
			MIMEType: "text/plain",
			Text:     "hello world",
		}
		blobRC := &mcpsdk.ResourceContents{
			URI:      "alluredeck://test/1",
			MIMEType: "image/png",
			Blob:     []byte("binarydata"),
		}
		r := &mcpsdk.ReadResourceResult{
			Contents: []*mcpsdk.ResourceContents{textRC, blobRC},
		}
		return r, nil
	})

	// Probe AddResourceTemplate method.
	s.AddResourceTemplate(&mcpsdk.ResourceTemplate{
		URITemplate: "alluredeck://attachment/{id}",
		Name:        "attachment",
	}, func(_ context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		return &mcpsdk.ReadResourceResult{}, nil
	})
}

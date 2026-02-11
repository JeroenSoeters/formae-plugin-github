package prov

import (
	"context"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
)

// Provisioner is the interface that all GitHub resource provisioners implement.
type Provisioner interface {
	Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error)
	Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error)
	Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error)
	Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error)
	Status(ctx context.Context, req *resource.StatusRequest) (*resource.StatusResult, error)
	List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error)
}

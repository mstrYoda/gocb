package gocb

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/pkg/errors"

	gocbcore "github.com/couchbase/gocbcore/v8"
)

// DesignDocumentNamespace represents which namespace a design document resides in.
type DesignDocumentNamespace bool

const (
	// ProductionDesignDocumentNamespace means that a design document resides in the production namespace.
	ProductionDesignDocumentNamespace = true

	// DevelopmentDesignDocumentNamespace means that a design document resides in the development namespace.
	DevelopmentDesignDocumentNamespace = false
)

// ViewIndexManager provides methods for performing View management.
// Volatile: This API is subject to change at any time.
type ViewIndexManager struct {
	bucketName string
	httpClient httpProvider
}

// View represents a Couchbase view within a design document.
type View struct {
	Map    string `json:"map,omitempty"`
	Reduce string `json:"reduce,omitempty"`
}

func (v View) hasReduce() bool {
	return v.Reduce != ""
}

// DesignDocument represents a Couchbase design document containing multiple views.
type DesignDocument struct {
	Name  string          `json:"-"`
	Views map[string]View `json:"views,omitempty"`
}

// GetDesignDocumentOptions is the set of options available to the ViewIndexManager GetDesignDocument operation.
type GetDesignDocumentOptions struct {
	Timeout time.Duration
	Context context.Context
}

func (vm *ViewIndexManager) ddocName(name string, isProd DesignDocumentNamespace) string {
	if isProd {
		if strings.HasPrefix(name, "dev_") {
			name = strings.TrimLeft(name, "dev_")
		}
	} else {
		if !strings.HasPrefix(name, "dev_") {
			name = "dev_" + name
		}
	}

	return name
}

// GetDesignDocument retrieves a single design document for the given bucket.
func (vm *ViewIndexManager) GetDesignDocument(name string, namespace DesignDocumentNamespace, opts *GetDesignDocumentOptions) (*DesignDocument, error) {
	if opts == nil {
		opts = &GetDesignDocumentOptions{}
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	name = vm.ddocName(name, namespace)

	req := &gocbcore.HttpRequest{
		Service: gocbcore.ServiceType(CapiService),
		Path:    fmt.Sprintf("/_design/%s", name),
		Method:  "GET",
		Context: ctx,
	}

	resp, err := vm.httpClient.DoHttpRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		err = resp.Body.Close()
		if err != nil {
			logDebugf("Failed to close socket (%s)", err)
		}

		return nil, viewIndexError{
			statusCode:   resp.StatusCode,
			message:      string(data),
			indexMissing: resp.StatusCode == 404,
		}
	}

	ddocObj := DesignDocument{}
	jsonDec := json.NewDecoder(resp.Body)
	err = jsonDec.Decode(&ddocObj)
	if err != nil {
		return nil, err
	}

	ddocObj.Name = strings.TrimPrefix(name, "dev_")
	return &ddocObj, nil
}

// GetAllDesignDocumentsOptions is the set of options available to the ViewIndexManager GetAllDesignDocuments operation.
type GetAllDesignDocumentsOptions struct {
	Timeout time.Duration
	Context context.Context
}

// GetAllDesignDocuments will retrieve all design documents for the given bucket.
func (vm *ViewIndexManager) GetAllDesignDocuments(namespace DesignDocumentNamespace, opts *GetAllDesignDocumentsOptions) ([]*DesignDocument, error) {
	if opts == nil {
		opts = &GetAllDesignDocumentsOptions{}
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	req := &gocbcore.HttpRequest{
		Service: gocbcore.ServiceType(MgmtService),
		Path:    fmt.Sprintf("/pools/default/buckets/%s/ddocs", vm.bucketName),
		Method:  "GET",
		Context: ctx,
	}

	resp, err := vm.httpClient.DoHttpRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		err = resp.Body.Close()
		if err != nil {
			logDebugf("Failed to close socket (%s)", err)
		}
		return nil, viewIndexError{statusCode: resp.StatusCode, message: string(data)}
	}

	var ddocsObj struct {
		Rows []struct {
			Doc struct {
				Meta struct {
					Id string
				}
				Json DesignDocument
			}
		}
	}
	jsonDec := json.NewDecoder(resp.Body)
	err = jsonDec.Decode(&ddocsObj)
	if err != nil {
		return nil, err
	}

	var ddocs []*DesignDocument
	for index, ddocData := range ddocsObj.Rows {
		ddoc := &ddocsObj.Rows[index].Doc.Json
		isProd := !strings.HasPrefix(ddoc.Name, "dev_")
		if isProd == bool(namespace) {
			ddoc.Name = strings.TrimPrefix(ddocData.Doc.Meta.Id[8:], "dev_")
			ddocs = append(ddocs, ddoc)
		}
	}

	return ddocs, nil
}

// UpsertDesignDocumentOptions is the set of options available to the ViewIndexManager UpsertDesignDocument operation.
type UpsertDesignDocumentOptions struct {
	Timeout time.Duration
	Context context.Context
}

// UpsertDesignDocument will insert a design document to the given bucket, or update
// an existing design document with the same name.
func (vm *ViewIndexManager) UpsertDesignDocument(ddoc DesignDocument, namespace DesignDocumentNamespace, opts *UpsertDesignDocumentOptions) error {
	if opts == nil {
		opts = &UpsertDesignDocumentOptions{}
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	data, err := json.Marshal(&ddoc)
	if err != nil {
		return err
	}

	ddoc.Name = vm.ddocName(ddoc.Name, namespace)

	req := &gocbcore.HttpRequest{
		Service: gocbcore.ServiceType(CapiService),
		Path:    fmt.Sprintf("/_design/%s", ddoc.Name),
		Method:  "PUT",
		Body:    data,
		Context: ctx,
	}

	resp, err := vm.httpClient.DoHttpRequest(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 201 {
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		err = resp.Body.Close()
		if err != nil {
			logDebugf("Failed to close socket (%s)", err)
		}
		return viewIndexError{statusCode: resp.StatusCode, message: string(data)}
	}

	return nil
}

// DropDesignDocumentOptions is the set of options available to the ViewIndexManager Upsert operation.
type DropDesignDocumentOptions struct {
	Timeout time.Duration
	Context context.Context
}

// DropDesignDocument will remove a design document from the given bucket.
func (vm *ViewIndexManager) DropDesignDocument(name string, namespace DesignDocumentNamespace, opts *DropDesignDocumentOptions) error {
	if opts == nil {
		opts = &DropDesignDocumentOptions{}
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	name = vm.ddocName(name, namespace)

	req := &gocbcore.HttpRequest{
		Service: gocbcore.ServiceType(CapiService),
		Path:    fmt.Sprintf("/_design/%s", name),
		Method:  "DELETE",
		Context: ctx,
	}

	resp, err := vm.httpClient.DoHttpRequest(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		err = resp.Body.Close()
		if err != nil {
			logDebugf("Failed to close socket (%s)", err)
		}
		return viewIndexError{
			statusCode:   resp.StatusCode,
			message:      string(data),
			indexMissing: resp.StatusCode == 404,
		}
	}

	return nil
}

// PublishDesignDocumentOptions is the set of options available to the ViewIndexManager PublishDesignDocument operation.
type PublishDesignDocumentOptions struct {
	Timeout time.Duration
	Context context.Context
}

// PublishDesignDocument publishes a design document to the given bucket.
func (vm *ViewIndexManager) PublishDesignDocument(name string, opts *PublishDesignDocumentOptions) error {
	if opts == nil {
		opts = &PublishDesignDocumentOptions{}
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	devdoc, err := vm.GetDesignDocument(name, false, &GetDesignDocumentOptions{
		Context: ctx,
	})
	if err != nil {
		indexErr, ok := err.(viewIndexError)
		if ok {
			if indexErr.indexMissing {
				return viewIndexError{message: "Development design document does not exist", indexMissing: true}
			}
		}
		return err
	}

	err = vm.UpsertDesignDocument(*devdoc, true, &UpsertDesignDocumentOptions{
		Context: ctx,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create ")
	}

	err = vm.DropDesignDocument(devdoc.Name, false, &DropDesignDocumentOptions{
		Context: ctx,
	})
	if err != nil {
		return viewIndexError{message: fmt.Sprintf("failed to drop development index: %v", err), publishDropFail: true}
	}

	return nil
}

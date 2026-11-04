package provider

import (
	"context"
	"fmt"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// Bucket resource
// ---------------------------------------------------------------------------

// Bucket is a Hub storage bucket (a mutable, non-versioned object store).
// Its files are runtime data and not managed here.
type Bucket struct{}

// BucketArgs are the inputs of a Bucket. The documented API creates, reads
// and deletes buckets only, so every property except the token replaces the
// bucket.
type BucketArgs struct {
	// Token authenticates with the Hugging Face API.
	Token *string `pulumi:"token,optional" provider:"secret"`
	// Namespace is the owning user or organization. Defaults to the token's user.
	Namespace *string `pulumi:"namespace,optional" provider:"replaceOnChanges"`
	// Name is the bucket name. Defaults to the Pulumi resource name.
	Name *string `pulumi:"name,optional" provider:"replaceOnChanges"`
	// Private controls visibility. Defaults to false.
	Private *bool `pulumi:"private,optional" provider:"replaceOnChanges"`
	// Region is where the data is stored ("us" or "eu"). Create-only.
	Region *string `pulumi:"region,optional" provider:"replaceOnChanges"`
	// ResourceGroupID is the Enterprise resource group. Create-only.
	ResourceGroupID *string `pulumi:"resourceGroupId,optional" provider:"replaceOnChanges"`
}

// BucketState is the stored state of a Bucket.
type BucketState struct {
	BucketArgs
	// BucketID is "namespace/name".
	BucketID string `pulumi:"bucketId"`
	// URL is the web URL of the bucket.
	URL string `pulumi:"url"`
}

func (r *Bucket) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Hugging Face Hub storage bucket. The resource ID is 'namespace/name'; import with it "+
		"(for example: pulumi import huggingface:index:Bucket b acme/artifacts). The documented API has no bucket "+
		"settings update, so changing visibility, region, resource group or name REPLACES the bucket and deletes its files. "+
		"Region and resource group are not returned by the API and stay as configured.")
}

func (a *BucketArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Namespace, "The owning user or organization. Defaults to the token's user. Changing it replaces the bucket.")
	ann.Describe(&a.Name, "The bucket name. Defaults to the Pulumi resource name. Changing it replaces the bucket.")
	ann.Describe(&a.Private, "Whether the bucket is private. Defaults to false. Changing it replaces the bucket and deletes its files.")
	ann.Describe(&a.Region, "Where the data is stored ('us' or 'eu'). Create-only; changing it replaces the bucket.")
	ann.Describe(&a.ResourceGroupID, "Enterprise resource group ID. Create-only; changing it replaces the bucket.")
}

func (s *BucketState) Annotate(ann infer.Annotator) {
	ann.Describe(&s.BucketID, "The bucket identifier: 'namespace/name'.")
	ann.Describe(&s.URL, "The web URL of the bucket.")
}

var (
	_ infer.CustomResource[BucketArgs, BucketState] = (*Bucket)(nil)
	_ infer.CustomDiff[BucketArgs, BucketState]     = (*Bucket)(nil)
	_ infer.CustomRead[BucketArgs, BucketState]     = (*Bucket)(nil)
	_ infer.CustomUpdate[BucketArgs, BucketState]   = (*Bucket)(nil)
	_ infer.CustomDelete[BucketState]               = (*Bucket)(nil)
)

func bucketURL(id string) string { return "https://huggingface.co/buckets/" + id }

func visibilityOf(private bool) string {
	if private {
		return "private"
	}
	return "public"
}

func (r *Bucket) Create(ctx context.Context, req infer.CreateRequest[BucketArgs]) (infer.CreateResponse[BucketState], error) {
	args := req.Inputs
	state := BucketState{BucketArgs: args}
	if req.DryRun {
		return infer.CreateResponse[BucketState]{ID: req.Name, Output: state}, nil
	}
	client, token, err := clientFor(ctx, args.Token)
	if err != nil {
		return infer.CreateResponse[BucketState]{}, err
	}
	ns, err := resolveNamespace(args.Namespace, token, client)
	if err != nil {
		return infer.CreateResponse[BucketState]{}, fmt.Errorf("resolve namespace: %w", err)
	}
	name := orDefault(args.Name, req.Name)
	payload := map[string]any{"visibility": visibilityOf(orDefault(args.Private, false))}
	if args.Region != nil {
		payload["region"] = *args.Region
	}
	if args.ResourceGroupID != nil {
		payload["resourceGroupId"] = *args.ResourceGroupID
	}
	if err := client.CreateBucket(ns, name, payload); err != nil {
		return infer.CreateResponse[BucketState]{}, fmt.Errorf("create bucket %q: %w", ns+"/"+name, err)
	}
	id := ns + "/" + name
	state.Namespace = ptr(ns)
	state.BucketID, state.URL = id, bucketURL(id)
	return infer.CreateResponse[BucketState]{ID: id, Output: state}, nil
}

func (r *Bucket) Read(ctx context.Context, req infer.ReadRequest[BucketArgs, BucketState]) (infer.ReadResponse[BucketArgs, BucketState], error) {
	type resp = infer.ReadResponse[BucketArgs, BucketState]
	prior := req.Inputs
	if prior.Token == nil {
		prior.Token = req.State.Token
	}
	ns, name, err := splitID(req.ID)
	if err != nil {
		return resp{}, err
	}
	client, _, err := clientFor(ctx, prior.Token)
	if err != nil {
		return resp{}, err
	}
	b, err := client.GetBucket(ns, name)
	if err != nil {
		return resp{}, fmt.Errorf("read bucket %q: %w", req.ID, err)
	}
	if b == nil {
		return resp{}, nil
	}
	imported := prior.Namespace == nil && prior.Name == nil && prior.Private == nil && req.State.BucketID == ""
	inputs := BucketArgs{
		Token: prior.Token, Namespace: prior.Namespace, Name: prior.Name,
		Region:          firstNonNil(prior.Region, req.State.Region),
		ResourceGroupID: firstNonNil(prior.ResourceGroupID, req.State.ResourceGroupID),
	}
	if imported {
		inputs.Namespace, inputs.Name = ptr(ns), ptr(name)
	}
	inputs.Private = refreshOpt(prior.Private, b.Private, true, imported, ptr(false))
	stateArgs := inputs
	stateArgs.Namespace = ptr(ns)
	stateArgs.Private = ptr(b.Private)
	return resp{
		ID:     req.ID,
		Inputs: inputs,
		State:  BucketState{BucketArgs: stateArgs, BucketID: req.ID, URL: bucketURL(req.ID)},
	}, nil
}

func (r *Bucket) Diff(_ context.Context, req infer.DiffRequest[BucketArgs, BucketState]) (infer.DiffResponse, error) {
	o, n := req.State.BucketArgs, req.Inputs
	d := newDiffSet()
	idNS, idName, _ := splitID(req.ID)
	d.replace("namespace", namespaceChanged(idNS, n.Namespace))
	d.replace("name", n.Name != nil && *n.Name != idName)
	d.replace("private", orDefault(o.Private, false) != orDefault(n.Private, false))
	d.replace("region", createOnlyChanged(o.Region, n.Region))
	d.replace("resourceGroupId", createOnlyChanged(o.ResourceGroupID, n.ResourceGroupID))
	d.update("token", orDefault(o.Token, "") != orDefault(n.Token, ""))
	return d.response(), nil
}

// Update only records a new token: everything else replaces the bucket.
func (r *Bucket) Update(_ context.Context, req infer.UpdateRequest[BucketArgs, BucketState]) (infer.UpdateResponse[BucketState], error) {
	st := req.State
	st.BucketArgs = req.Inputs
	st.Namespace = req.State.Namespace
	return infer.UpdateResponse[BucketState]{Output: st}, nil
}

func (r *Bucket) Delete(ctx context.Context, req infer.DeleteRequest[BucketState]) (infer.DeleteResponse, error) {
	ns, name, err := splitID(req.ID)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	client, _, err := clientFor(ctx, req.State.Token)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := client.DeleteBucket(ns, name); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete bucket %q: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}

// ---------------------------------------------------------------------------
// getBucket
// ---------------------------------------------------------------------------

// GetBucket looks up a bucket.
type GetBucket struct{}

// GetBucketArgs selects the bucket.
type GetBucketArgs struct {
	Token     *string `pulumi:"token,optional" provider:"secret"`
	Namespace *string `pulumi:"namespace,optional"`
	Name      string  `pulumi:"name"`
}

// GetBucketResult is the looked-up bucket.
type GetBucketResult struct {
	BucketID   string `pulumi:"bucketId"`
	Namespace  string `pulumi:"namespace"`
	Name       string `pulumi:"name"`
	Private    bool   `pulumi:"private"`
	CreatedAt  string `pulumi:"createdAt"`
	Size       int    `pulumi:"size"`
	TotalFiles int    `pulumi:"totalFiles"`
	URL        string `pulumi:"url"`
}

func (f *GetBucket) Annotate(a infer.Annotator) {
	a.Describe(&f, "Look up a Hugging Face Hub storage bucket by namespace and name.")
}

func (a *GetBucketArgs) Annotate(ann infer.Annotator) {
	ann.Describe(&a.Token, "Hugging Face API token. Falls back to the provider token.")
	ann.Describe(&a.Namespace, "The owning user or organization. Defaults to the token's user.")
	ann.Describe(&a.Name, "The bucket name.")
}

func (r *GetBucketResult) Annotate(ann infer.Annotator) {
	ann.Describe(&r.BucketID, "The bucket identifier: 'namespace/name'.")
	ann.Describe(&r.Namespace, "The owning user or organization.")
	ann.Describe(&r.Name, "The bucket name.")
	ann.Describe(&r.Private, "Whether the bucket is private.")
	ann.Describe(&r.CreatedAt, "When the bucket was created.")
	ann.Describe(&r.Size, "Total size in bytes.")
	ann.Describe(&r.TotalFiles, "Number of files.")
	ann.Describe(&r.URL, "The web URL of the bucket.")
}

func (f *GetBucket) Invoke(ctx context.Context, req infer.FunctionRequest[GetBucketArgs]) (infer.FunctionResponse[GetBucketResult], error) {
	out := infer.FunctionResponse[GetBucketResult]{}
	if req.Input.Name == "" {
		return out, fmt.Errorf("huggingface: getBucket needs a name")
	}
	client, token, err := clientFor(ctx, req.Input.Token)
	if err != nil {
		return out, err
	}
	ns, err := resolveNamespace(req.Input.Namespace, token, client)
	if err != nil {
		return out, fmt.Errorf("huggingface: resolve namespace: %w", err)
	}
	b, err := client.GetBucket(ns, req.Input.Name)
	if err != nil {
		return out, fmt.Errorf("huggingface: look up bucket %q: %w", ns+"/"+req.Input.Name, err)
	}
	if b == nil {
		return out, fmt.Errorf("huggingface: bucket %q was not found", ns+"/"+req.Input.Name)
	}
	id := ns + "/" + req.Input.Name
	out.Output = GetBucketResult{BucketID: id, Namespace: ns, Name: req.Input.Name, Private: b.Private, CreatedAt: b.CreatedAt,
		Size: int(b.Size), TotalFiles: int(b.TotalFiles), URL: bucketURL(id)}
	return out, nil
}

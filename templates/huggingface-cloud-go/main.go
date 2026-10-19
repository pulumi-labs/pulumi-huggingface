package main

import (
	"github.com/pulumi/pulumi-huggingface/sdk/go/huggingface"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		isPrivate := cfg.GetBool("private")

		// Build optional namespace argument — only set when non-empty so the API
		// falls back to the token owner's account by default.
		var namespace pulumi.StringPtrInput
		if ns := cfg.Get("namespace"); ns != "" {
			namespace = pulumi.StringPtr(ns)
		}

		// Create a model repository on the Hugging Face Hub.
		// The repository stores model weights, config, and any other artifacts.
		repo, err := huggingface.NewRepository(ctx, "hf-repo", &huggingface.RepositoryArgs{
			Namespace: namespace,
			RepoType:  pulumi.StringPtr("model"),
			Private:   pulumi.BoolPtr(isPrivate),
		})
		if err != nil {
			return err
		}

		// Create a Gradio Space on free cpu-basic hardware to showcase the model.
		// cpu-basic Spaces are free on the Hub and ideal for lightweight demos.
		space, err := huggingface.NewSpace(ctx, "hf-space", &huggingface.SpaceArgs{
			Namespace: namespace,
			Sdk:       pulumi.String("gradio"),
			Hardware:  pulumi.StringPtr("cpu-basic"),
			Private:   pulumi.BoolPtr(isPrivate),
		})
		if err != nil {
			return err
		}

		ctx.Export("repoId", repo.RepoId)
		ctx.Export("repoUrl", repo.Url)
		ctx.Export("spaceId", space.SpaceId)
		ctx.Export("spaceUrl", space.Url)

		return nil
	})
}

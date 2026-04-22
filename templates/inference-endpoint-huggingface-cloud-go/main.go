package main

import (
	"github.com/pulumi/pulumi-huggingface/sdk/go/huggingface"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		hfToken := cfg.RequireSecret("hfToken")
		modelRepo := cfg.Get("modelRepository")
		modelTask := cfg.Get("modelTask")
		vendor := cfg.Get("vendor")
		region := cfg.Get("region")
		instanceType := cfg.Get("instanceType")
		instanceSize := cfg.Get("instanceSize")
		endpointType := cfg.Get("endpointType")

		// Apply defaults for optional config values.
		if modelRepo == "" {
			modelRepo = "mistralai/Mistral-7B-Instruct-v0.3"
		}
		if modelTask == "" {
			modelTask = "text-generation"
		}
		if vendor == "" {
			vendor = "aws"
		}
		if region == "" {
			region = "us-east-1"
		}
		if instanceType == "" {
			instanceType = "nvidia-a10g"
		}
		if instanceSize == "" {
			instanceSize = "x1"
		}
		if endpointType == "" {
			endpointType = "protected"
		}

		// Build optional namespace argument.
		var namespace pulumi.StringPtrInput
		if ns := cfg.Get("namespace"); ns != "" {
			namespace = pulumi.StringPtr(ns)
		}

		// Explicit provider so credentials come from project-namespace config,
		// not environment variables.
		provider, err := huggingface.NewProvider(ctx, "huggingface", &huggingface.ProviderArgs{
			Token: hfToken,
		})
		if err != nil {
			return err
		}

		minReplica := 0
		maxReplica := 2
		scaleToZeroTimeout := 15

		// Deploy a dedicated GPU inference endpoint.
		// minReplica=0 enables scale-to-zero so idle time costs nothing.
		endpoint, err := huggingface.NewInferenceEndpoint(ctx, "hf-endpoint", &huggingface.InferenceEndpointArgs{
			Namespace:          namespace,
			ModelRepository:    pulumi.String(modelRepo),
			ModelFramework:     pulumi.String("pytorch"),
			ModelTask:          pulumi.String(modelTask),
			Vendor:             pulumi.String(vendor),
			Region:             pulumi.String(region),
			Accelerator:        pulumi.String("gpu"),
			InstanceType:       pulumi.String(instanceType),
			InstanceSize:       pulumi.String(instanceSize),
			MinReplica:         pulumi.IntPtr(minReplica),
			MaxReplica:         pulumi.IntPtr(maxReplica),
			ScaleToZeroTimeout: pulumi.IntPtr(scaleToZeroTimeout),
			EndpointType:       pulumi.StringPtr(endpointType),
		}, pulumi.Provider(provider))
		if err != nil {
			return err
		}

		// Gradio Space wired to the endpoint via environment variable injection.
		// cpu-basic hardware keeps the UI free while compute cost sits on the endpoint.
		space, err := huggingface.NewSpace(ctx, "hf-space", &huggingface.SpaceArgs{
			Namespace: namespace,
			Sdk:       pulumi.String("gradio"),
			Hardware:  pulumi.StringPtr("cpu-basic"),
			Variables: huggingface.SpaceVariableInputArray{
				huggingface.SpaceVariableInputArgs{
					Key:   pulumi.String("ENDPOINT_URL"),
					Value: endpoint.Url,
				},
				huggingface.SpaceVariableInputArgs{
					Key:   pulumi.String("MODEL_NAME"),
					Value: pulumi.String(modelRepo),
				},
			},
			Secrets: huggingface.SpaceSecretInputArray{
				huggingface.SpaceSecretInputArgs{
					Key:   pulumi.String("HF_TOKEN"),
					Value: hfToken,
				},
			},
		}, pulumi.Provider(provider))
		if err != nil {
			return err
		}

		ctx.Export("endpointUrl", endpoint.Url)
		ctx.Export("endpointStatus", endpoint.Status)
		ctx.Export("spaceUrl", space.Url)

		return nil
	})
}

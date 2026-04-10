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

		// Deploy Mistral 7B with TGI container for high-throughput text generation
		endpoint, err := huggingface.NewInferenceEndpoint(ctx, "mistral-7b", &huggingface.InferenceEndpointArgs{
			Token:           hfToken,
			ModelRepository: pulumi.String("mistralai/Mistral-7B-Instruct-v0.3"),
			ModelFramework:  pulumi.String("pytorch"),
			ModelTask:       pulumi.String("text-generation"),
			// Deploy with TGI for production throughput
			CustomImageUrl: pulumi.String("ghcr.io/huggingface/text-generation-inference:2.4.0"),
			CustomImageEnvVars: pulumi.StringMap{
				"MAX_BATCH_TOTAL_TOKENS": pulumi.String("32768"),
				"MAX_INPUT_LENGTH":       pulumi.String("4096"),
			},
			Vendor:       pulumi.String("aws"),
			Region:       pulumi.String("us-east-1"),
			Accelerator:  pulumi.String("gpu"),
			InstanceType: pulumi.String("nvidia-a10g"),
			InstanceSize: pulumi.String("x1"),
			MinReplica:   pulumi.IntPtr(1),
			MaxReplica:   pulumi.IntPtr(4),
			EndpointType: pulumi.StringPtr("protected"),
		})
		if err != nil {
			return err
		}

		ctx.Export("endpointUrl", endpoint.Url)
		ctx.Export("endpointStatus", endpoint.Status)
		return nil
	})
}

using Pulumi;
using Pulumi.Huggingface;
using Pulumi.Huggingface.Inputs;
using System.Collections.Generic;

return await Deployment.RunAsync(() =>
{
    // Use global::Pulumi.Config to avoid ambiguity with Pulumi.Huggingface.Config.
    var config          = new global::Pulumi.Config();
    var hfToken         = config.RequireSecret("hfToken");
    var ns              = config.Get("namespace");
    var modelRepository = config.Get("modelRepository") ?? "mistralai/Mistral-7B-Instruct-v0.3";
    var modelTask       = config.Get("modelTask")       ?? "text-generation";
    var vendor          = config.Get("vendor")          ?? "aws";
    var region          = config.Get("region")          ?? "us-east-1";
    var instanceType    = config.Get("instanceType")    ?? "nvidia-a10g";
    var instanceSize    = config.Get("instanceSize")    ?? "x1";
    var endpointType    = config.Get("endpointType")    ?? "protected";

    // Explicit provider so credentials come from project-namespace config,
    // not environment variables.
    var provider = new Provider("huggingface", new ProviderArgs
    {
        Token = hfToken,
    });

    // Build optional namespace argument.
    var nsArg = string.IsNullOrEmpty(ns) ? null : ns;

    // Deploy a dedicated GPU inference endpoint.
    // MinReplica = 0 enables scale-to-zero so idle time costs nothing.
    var endpoint = new InferenceEndpoint("hf-endpoint", new InferenceEndpointArgs
    {
        Namespace          = nsArg,
        ModelRepository    = modelRepository,
        ModelFramework     = "pytorch",
        ModelTask          = modelTask,
        Vendor             = vendor,
        Region             = region,
        Accelerator        = "gpu",
        InstanceType       = instanceType,
        InstanceSize       = instanceSize,
        MinReplica         = 0,
        MaxReplica         = 2,
        ScaleToZeroTimeout = 15,
        EndpointType       = endpointType,
    }, new CustomResourceOptions { Provider = provider });

    // Gradio Space wired to the endpoint via environment variable injection.
    // cpu-basic hardware keeps the UI free while compute cost sits on the endpoint.
    var space = new Space("hf-space", new SpaceArgs
    {
        Namespace = nsArg,
        Sdk       = "gradio",
        Hardware  = "cpu-basic",
        Variables = new[]
        {
            new SpaceVariableInputArgs { Key = "ENDPOINT_URL", Value = endpoint.Url },
            new SpaceVariableInputArgs { Key = "MODEL_NAME",   Value = modelRepository },
        },
        Secrets = new[]
        {
            new SpaceSecretInputArgs { Key = "HF_TOKEN", Value = hfToken },
        },
    }, new CustomResourceOptions { Provider = provider });

    return new Dictionary<string, object?>
    {
        ["endpointUrl"]    = endpoint.Url,
        ["endpointStatus"] = endpoint.Status,
        ["spaceUrl"]       = space.Url,
    };
});

using Pulumi;
using Pulumi.Huggingface;
using System.Collections.Generic;

return await Deployment.RunAsync(() =>
{
    var config = new Config();
    var hfToken = config.RequireSecret("hfToken");

    // Deploy a sentence-similarity model endpoint for semantic search
    var endpoint = new InferenceEndpoint("e5-large", new InferenceEndpointArgs
    {
        Token = hfToken,
        ModelRepository = "intfloat/e5-large-v2",
        ModelFramework = "pytorch",
        ModelTask = "feature-extraction",
        Vendor = "aws",
        Region = "us-east-1",
        Accelerator = "cpu",
        InstanceType = "intel-icl",
        InstanceSize = "x2",
        MinReplica = 1,
        MaxReplica = 2,
        EndpointType = "protected",
    });

    return new Dictionary<string, object?>
    {
        ["endpointUrl"] = endpoint.Url,
        ["endpointStatus"] = endpoint.Status,
    };
});

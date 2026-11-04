import * as pulumi from "@pulumi/pulumi";
import * as huggingface from "@pulumi-labs/huggingface";

const config = new pulumi.Config();
// Only used as the Space HF_TOKEN secret below; the provider reads huggingface:token or HUGGINGFACE_TOKEN.
const hfToken = config.requireSecret("hfToken");

// Deploy Llama 3 8B to a dedicated GPU inference endpoint
const endpoint = new huggingface.InferenceEndpoint("llama-3-8b", {
    modelRepository: "meta-llama/Meta-Llama-3-8B-Instruct",
    modelFramework: "pytorch",
    modelTask: "text-generation",
    vendor: "aws",
    region: "us-east-1",
    accelerator: "gpu",
    instanceType: "nvidia-a10g",
    instanceSize: "x1",
    // Scale to zero after 60 minutes of inactivity to control costs
    minReplica: 0,
    maxReplica: 2,
    scaleToZeroTimeout: 60,
    endpointType: "authenticated",
});

// Create a Gradio demo Space that calls the endpoint
const demo = new huggingface.Space("llama-demo", {
    sdk: "gradio",
    hardware: "cpu-basic",
    variables: [
        { key: "ENDPOINT_URL", value: endpoint.url },
        { key: "MODEL_NAME", value: "meta-llama/Meta-Llama-3-8B-Instruct" },
    ],
    secrets: [
        { key: "HF_TOKEN", value: hfToken },
    ],
});

// Export the inference URL and Space URL
export const endpointUrl = endpoint.url;
export const endpointStatus = endpoint.status;
export const spaceUrl = demo.url;

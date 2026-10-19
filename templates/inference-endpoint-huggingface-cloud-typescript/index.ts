import * as pulumi from "@pulumi/pulumi";
import * as huggingface from "@pulumi/huggingface";

const config          = new pulumi.Config();
const hfToken         = config.requireSecret("hfToken");
const namespace       = config.get("namespace") || undefined;
const modelRepository = config.get("modelRepository") || "mistralai/Mistral-7B-Instruct-v0.3";
const modelTask       = config.get("modelTask")       || "text-generation";
const vendor          = config.get("vendor")          || "aws";
const region          = config.get("region")          || "us-east-1";
const instanceType    = config.get("instanceType")    || "nvidia-a10g";
const instanceSize    = config.get("instanceSize")    || "x1";
const endpointType    = config.get("endpointType")    || "protected";

// Deploy a dedicated GPU inference endpoint.
// minReplica: 0 enables scale-to-zero so idle time costs nothing.
const endpoint = new huggingface.InferenceEndpoint("hf-endpoint", {
    namespace,
    modelRepository,
    modelFramework: "pytorch",
    modelTask,
    vendor,
    region,
    accelerator:        "gpu",
    instanceType,
    instanceSize,
    minReplica:         0,
    maxReplica:         2,
    scaleToZeroTimeout: 15,
    endpointType,
});

// Gradio Space wired to the endpoint via environment variable injection.
// cpu-basic hardware keeps the UI free while compute cost sits on the endpoint.
const space = new huggingface.Space("hf-space", {
    namespace,
    sdk:      "gradio",
    hardware: "cpu-basic",
    variables: [
        { key: "ENDPOINT_URL", value: endpoint.url },
        { key: "MODEL_NAME",   value: modelRepository },
    ],
    secrets: [
        { key: "HF_TOKEN", value: hfToken },
    ],
});

export const endpointUrl    = endpoint.url;
export const endpointStatus = endpoint.status;
export const spaceUrl       = space.url;

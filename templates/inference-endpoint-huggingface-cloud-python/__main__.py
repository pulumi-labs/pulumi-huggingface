import pulumi
import pulumi_huggingface as huggingface

config           = pulumi.Config()
hf_token         = config.require_secret("hfToken")
namespace        = config.get("namespace") or None
model_repository = config.get("modelRepository") or "mistralai/Mistral-7B-Instruct-v0.3"
model_task       = config.get("modelTask")       or "text-generation"
vendor           = config.get("vendor")          or "aws"
region           = config.get("region")          or "us-east-1"
instance_type    = config.get("instanceType")    or "nvidia-a10g"
instance_size    = config.get("instanceSize")    or "x1"
endpoint_type    = config.get("endpointType")    or "protected"

# Explicit provider so credentials come from project-namespace config,
# not environment variables.
provider = huggingface.Provider("huggingface", token=hf_token)

# Deploy a dedicated GPU inference endpoint.
# min_replica=0 enables scale-to-zero so idle time costs nothing.
endpoint = huggingface.InferenceEndpoint(
    "hf-endpoint",
    namespace=namespace,
    model_repository=model_repository,
    model_framework="pytorch",
    model_task=model_task,
    vendor=vendor,
    region=region,
    accelerator="gpu",
    instance_type=instance_type,
    instance_size=instance_size,
    min_replica=0,
    max_replica=2,
    scale_to_zero_timeout=15,
    endpoint_type=endpoint_type,
    opts=pulumi.ResourceOptions(provider=provider),
)

# Gradio Space wired to the endpoint via environment variable injection.
# cpu-basic hardware keeps the UI free while compute cost sits on the endpoint.
space = huggingface.Space(
    "hf-space",
    namespace=namespace,
    sdk="gradio",
    hardware="cpu-basic",
    variables=[
        huggingface.SpaceVariableInputArgs(key="ENDPOINT_URL", value=endpoint.url),
        huggingface.SpaceVariableInputArgs(key="MODEL_NAME",   value=model_repository),
    ],
    secrets=[
        huggingface.SpaceSecretInputArgs(key="HF_TOKEN", value=hf_token),
    ],
    opts=pulumi.ResourceOptions(provider=provider),
)

pulumi.export("endpointUrl",    endpoint.url)
pulumi.export("endpointStatus", endpoint.status)
pulumi.export("spaceUrl",       space.url)

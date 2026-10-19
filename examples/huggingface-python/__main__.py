import pulumi
import pulumi_huggingface as huggingface

# Deploy a text-embedding model for RAG pipelines
endpoint = huggingface.InferenceEndpoint(
    "bge-large",
    model_repository="BAAI/bge-large-en-v1.5",
    model_framework="pytorch",
    model_task="feature-extraction",
    vendor="aws",
    region="us-east-1",
    accelerator="cpu",
    instance_type="intel-icl",
    instance_size="x4",
    min_replica=1,
    max_replica=3,
    endpoint_type="protected",
)

# Create a dataset repository for storing embeddings metadata
dataset_repo = huggingface.Repository(
    "embeddings-metadata",
    repo_type="dataset",
    private=True,
)

pulumi.export("endpoint_url", endpoint.url)
pulumi.export("endpoint_status", endpoint.status)
pulumi.export("dataset_repo_url", dataset_repo.url)

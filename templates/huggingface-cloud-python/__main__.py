import pulumi
import pulumi_huggingface as huggingface

config     = pulumi.Config()
namespace  = config.get("namespace") or None
is_private = config.get_bool("private") or False

# Create a model repository on the Hugging Face Hub.
# The repository stores model weights, config, and any other artifacts.
repo = huggingface.Repository(
    "hf-repo",
    namespace=namespace,
    repo_type="model",
    private=is_private,
)

# Create a Gradio Space on free cpu-basic hardware to showcase the model.
# cpu-basic Spaces are free on the Hub and ideal for lightweight demos.
space = huggingface.Space(
    "hf-space",
    namespace=namespace,
    sdk="gradio",
    hardware="cpu-basic",
    private=is_private,
)

pulumi.export("repoId",   repo.repo_id)
pulumi.export("repoUrl",  repo.url)
pulumi.export("spaceId",  space.space_id)
pulumi.export("spaceUrl", space.url)

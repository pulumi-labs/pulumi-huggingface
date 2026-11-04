import * as pulumi from "@pulumi/pulumi";
import * as huggingface from "@pulumi-labs/huggingface";

const config = new pulumi.Config();
const namespace = config.get("namespace") || undefined;
const isPrivate = config.getBoolean("private") ?? false;

// Create a model repository on the Hugging Face Hub.
// The repository stores model weights, config, and any other artifacts.
const repo = new huggingface.Repository("hf-repo", {
    namespace,
    repoType: "model",
    private:  isPrivate,
});

// Create a Gradio Space on free cpu-basic hardware to showcase the model.
// cpu-basic Spaces are free on the Hub and ideal for lightweight demos.
const space = new huggingface.Space("hf-space", {
    namespace,
    sdk:      "gradio",
    hardware: "cpu-basic",
    private:  isPrivate,
});

export const repoId   = repo.repoId;
export const repoUrl  = repo.url;
export const spaceId  = space.spaceId;
export const spaceUrl = space.url;

using Pulumi;
using Pulumi.Labs.Huggingface;
using System.Collections.Generic;

return await Deployment.RunAsync(() =>
{
    var config    = new Pulumi.Config();
    var ns        = config.Get("namespace");
    var isPrivate = config.GetBoolean("private") ?? false;

    // Build optional namespace argument — only set when non-empty so the API
    // falls back to the token owner's account by default.
    var nsArg = string.IsNullOrEmpty(ns) ? null : ns;

    // Create a model repository on the Hugging Face Hub.
    // The repository stores model weights, config, and any other artifacts.
    var repo = new Repository("hf-repo", new RepositoryArgs
    {
        Namespace = nsArg,
        RepoType  = "model",
        Private   = isPrivate,
    });

    // Create a Gradio Space on free cpu-basic hardware to showcase the model.
    // cpu-basic Spaces are free on the Hub and ideal for lightweight demos.
    var space = new Space("hf-space", new SpaceArgs
    {
        Namespace = nsArg,
        Sdk       = "gradio",
        Hardware  = "cpu-basic",
        Private   = isPrivate,
    });

    return new Dictionary<string, object?>
    {
        ["repoId"]   = repo.RepoId,
        ["repoUrl"]  = repo.Url,
        ["spaceId"]  = space.SpaceId,
        ["spaceUrl"] = space.Url,
    };
});

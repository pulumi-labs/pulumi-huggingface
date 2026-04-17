using Pulumi;
using Pulumi.Huggingface;
using System.Collections.Generic;

return await Deployment.RunAsync(() =>
{
    var config    = new Config();
    var hfToken   = config.RequireSecret("hfToken");
    var ns        = config.Get("namespace");
    var isPrivate = config.GetBoolean("private") ?? false;

    // Explicit provider so credentials come from project-namespace config,
    // not environment variables.
    var provider = new Provider("huggingface", new ProviderArgs
    {
        Token = hfToken,
    });

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
    }, new CustomResourceOptions { Provider = provider });

    // Create a Gradio Space on free cpu-basic hardware to showcase the model.
    // cpu-basic Spaces are free on the Hub and ideal for lightweight demos.
    var space = new Space("hf-space", new SpaceArgs
    {
        Namespace = nsArg,
        Sdk       = "gradio",
        Hardware  = "cpu-basic",
        Private   = isPrivate,
    }, new CustomResourceOptions { Provider = provider });

    return new Dictionary<string, object?>
    {
        ["repoId"]   = repo.RepoId,
        ["repoUrl"]  = repo.Url,
        ["spaceId"]  = space.SpaceId,
        ["spaceUrl"] = space.Url,
    };
});

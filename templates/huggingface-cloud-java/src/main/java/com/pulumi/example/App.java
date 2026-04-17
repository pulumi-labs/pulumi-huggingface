package com.pulumi.example;

import com.pulumi.Context;
import com.pulumi.Pulumi;
import com.pulumi.huggingface.Provider;
import com.pulumi.huggingface.ProviderArgs;
import com.pulumi.huggingface.Repository;
import com.pulumi.huggingface.RepositoryArgs;
import com.pulumi.huggingface.Space;
import com.pulumi.huggingface.SpaceArgs;
import com.pulumi.resources.CustomResourceOptions;

public class App {
    public static void main(String[] args) {
        Pulumi.run(App::stack);
    }

    private static void stack(Context ctx) {
        var config    = ctx.config();
        var hfToken   = config.requireSecret("hfToken");
        var namespace = config.get("namespace").orElse("");
        var isPrivate = config.getBoolean("private").orElse(false);

        // Explicit provider so credentials come from project-namespace config,
        // not environment variables.
        var provider = new Provider("huggingface", ProviderArgs.builder()
            .token(hfToken)
            .build());

        // Build optional namespace argument — only set when non-empty so the API
        // falls back to the token owner's account by default.
        var nsArg = namespace.isEmpty() ? null : namespace;

        // Create a model repository on the Hugging Face Hub.
        // The repository stores model weights, config, and any other artifacts.
        var repo = new Repository("hf-repo", RepositoryArgs.builder()
            .namespace(nsArg)
            .repoType("model")
            .private_(isPrivate)
            .build(),
            CustomResourceOptions.builder()
                .provider(provider)
                .build());

        // Create a Gradio Space on free cpu-basic hardware to showcase the model.
        // cpu-basic Spaces are free on the Hub and ideal for lightweight demos.
        var space = new Space("hf-space", SpaceArgs.builder()
            .namespace(nsArg)
            .sdk("gradio")
            .hardware("cpu-basic")
            .private_(isPrivate)
            .build(),
            CustomResourceOptions.builder()
                .provider(provider)
                .build());

        ctx.export("repoId",   repo.repoId());
        ctx.export("repoUrl",  repo.url());
        ctx.export("spaceId",  space.spaceId());
        ctx.export("spaceUrl", space.url());
    }
}

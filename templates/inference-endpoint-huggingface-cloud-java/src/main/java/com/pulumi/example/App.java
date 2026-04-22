package com.pulumi.example;

import com.pulumi.Context;
import com.pulumi.Pulumi;
import com.pulumi.huggingface.Provider;
import com.pulumi.huggingface.ProviderArgs;
import com.pulumi.huggingface.InferenceEndpoint;
import com.pulumi.huggingface.InferenceEndpointArgs;
import com.pulumi.huggingface.Space;
import com.pulumi.huggingface.SpaceArgs;
import com.pulumi.huggingface.inputs.SpaceVariableInputArgs;
import com.pulumi.huggingface.inputs.SpaceSecretInputArgs;
import com.pulumi.resources.CustomResourceOptions;

public class App {
    public static void main(String[] args) {
        Pulumi.run(App::stack);
    }

    private static void stack(Context ctx) {
        var config          = ctx.config();
        var hfToken         = config.requireSecret("hfToken");
        var namespace       = config.get("namespace").orElse("");
        var modelRepository = config.get("modelRepository").orElse("mistralai/Mistral-7B-Instruct-v0.3");
        var modelTask       = config.get("modelTask").orElse("text-generation");
        var vendor          = config.get("vendor").orElse("aws");
        var region          = config.get("region").orElse("us-east-1");
        var instanceType    = config.get("instanceType").orElse("nvidia-a10g");
        var instanceSize    = config.get("instanceSize").orElse("x1");
        var endpointType    = config.get("endpointType").orElse("protected");

        // Explicit provider so credentials come from project-namespace config,
        // not environment variables.
        var provider = new Provider("huggingface", ProviderArgs.builder()
            .token(hfToken)
            .build());

        // Build optional namespace argument.
        var nsArg = namespace.isEmpty() ? null : namespace;

        // Deploy a dedicated GPU inference endpoint.
        // minReplica=0 enables scale-to-zero so idle time costs nothing.
        var endpoint = new InferenceEndpoint("hf-endpoint", InferenceEndpointArgs.builder()
            .namespace(nsArg)
            .modelRepository(modelRepository)
            .modelFramework("pytorch")
            .modelTask(modelTask)
            .vendor(vendor)
            .region(region)
            .accelerator("gpu")
            .instanceType(instanceType)
            .instanceSize(instanceSize)
            .minReplica(0)
            .maxReplica(2)
            .scaleToZeroTimeout(15)
            .endpointType(endpointType)
            .build(),
            CustomResourceOptions.builder()
                .provider(provider)
                .build());

        // Gradio Space wired to the endpoint via environment variable injection.
        // cpu-basic hardware keeps the UI free while compute cost sits on the endpoint.
        var space = new Space("hf-space", SpaceArgs.builder()
            .namespace(nsArg)
            .sdk("gradio")
            .hardware("cpu-basic")
            .variables(
                SpaceVariableInputArgs.builder().key("ENDPOINT_URL").value(endpoint.url()).build(),
                SpaceVariableInputArgs.builder().key("MODEL_NAME").value(modelRepository).build()
            )
            .secrets(
                SpaceSecretInputArgs.builder().key("HF_TOKEN").value(hfToken).build()
            )
            .build(),
            CustomResourceOptions.builder()
                .provider(provider)
                .build());

        ctx.export("endpointUrl",    endpoint.url());
        ctx.export("endpointStatus", endpoint.status());
        ctx.export("spaceUrl",       space.url());
    }
}

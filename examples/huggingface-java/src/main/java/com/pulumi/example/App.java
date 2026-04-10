package com.pulumi.example;

import com.pulumi.Pulumi;
import com.pulumi.core.Output;
import com.pulumi.huggingface.InferenceEndpoint;
import com.pulumi.huggingface.InferenceEndpointArgs;

public class App {
    public static void main(String[] args) {
        Pulumi.run(ctx -> {
            var config = ctx.config();
            var hfToken = config.requireSecret("hfToken");

            // Deploy a summarization model endpoint
            var endpoint = new InferenceEndpoint("bart-summarizer",
                InferenceEndpointArgs.builder()
                    .token(hfToken)
                    .modelRepository("facebook/bart-large-cnn")
                    .modelFramework("pytorch")
                    .modelTask("summarization")
                    .vendor("aws")
                    .region("us-east-1")
                    .accelerator("cpu")
                    .instanceType("intel-icl")
                    .instanceSize("x2")
                    .minReplica(1)
                    .maxReplica(2)
                    .endpointType("protected")
                    .build());

            ctx.export("endpointUrl", endpoint.url());
            ctx.export("endpointStatus", endpoint.status());
        });
    }
}

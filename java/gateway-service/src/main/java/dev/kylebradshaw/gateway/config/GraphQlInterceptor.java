package dev.kylebradshaw.gateway.config;

import org.springframework.graphql.server.WebGraphQlInterceptor;
import org.springframework.graphql.server.WebGraphQlRequest;
import org.springframework.graphql.server.WebGraphQlResponse;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.stereotype.Component;
import reactor.core.publisher.Mono;

@Component
public class GraphQlInterceptor implements WebGraphQlInterceptor {

    @Override
    public Mono<WebGraphQlResponse> intercept(WebGraphQlRequest request, Chain chain) {
        String userId = null;
        String authorizationHeader = null;

        var auth = SecurityContextHolder.getContext().getAuthentication();
        if (auth != null && auth.getPrincipal() instanceof String principal) {
            userId = principal;
        }

        // Extract raw Authorization header for forwarding to downstream services
        var authHeaders = request.getHeaders().get("Authorization");
        if (authHeaders != null && !authHeaders.isEmpty()) {
            authorizationHeader = authHeaders.getFirst();
        }

        if (userId != null) {
            String finalUserId = userId;
            String finalAuthHeader = authorizationHeader;
            request.configureExecutionInput((input, builder) ->
                    builder.graphQLContext(ctx -> {
                        ctx.put("userId", finalUserId);
                        if (finalAuthHeader != null) {
                            ctx.put("authorizationHeader", finalAuthHeader);
                        }
                    }).build());
        }

        return chain.next(request);
    }
}

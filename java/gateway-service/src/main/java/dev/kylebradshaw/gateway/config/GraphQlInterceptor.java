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

        var auth = SecurityContextHolder.getContext().getAuthentication();
        if (auth != null && auth.getPrincipal() instanceof String principal) {
            userId = principal;
        }

        // Fallback: check X-User-Id header (for gateway-to-gateway or testing)
        if (userId == null) {
            var headerValues = request.getHeaders().get("X-User-Id");
            if (headerValues != null && !headerValues.isEmpty()) {
                userId = headerValues.getFirst();
            }
        }

        if (userId != null) {
            String finalUserId = userId;
            request.configureExecutionInput((input, builder) ->
                    builder.graphQLContext(ctx -> ctx.put("userId", finalUserId)).build());
        }

        return chain.next(request);
    }
}

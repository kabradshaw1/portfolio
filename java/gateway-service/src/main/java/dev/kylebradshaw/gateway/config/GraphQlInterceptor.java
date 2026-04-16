package dev.kylebradshaw.gateway.config;

import org.springframework.graphql.server.WebGraphQlInterceptor;
import org.springframework.graphql.server.WebGraphQlRequest;
import org.springframework.graphql.server.WebGraphQlResponse;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.stereotype.Component;
import reactor.core.publisher.Mono;

@Component
public class GraphQlInterceptor implements WebGraphQlInterceptor {

    private static final String ACCESS_TOKEN_COOKIE_PREFIX = "access_token=";

    @Override
    public Mono<WebGraphQlResponse> intercept(WebGraphQlRequest request, Chain chain) {
        String userId = null;
        String authorizationHeader = null;

        var auth = SecurityContextHolder.getContext().getAuthentication();
        if (auth != null && auth.getPrincipal() instanceof String principal) {
            userId = principal;
        }

        // Extract the Authorization header so downstream REST calls can forward it.
        // Fall back to the access_token cookie (httpOnly cookie auth flow) by
        // synthesizing "Bearer <token>" — downstream services accept either form.
        var authHeaders = request.getHeaders().get("Authorization");
        if (authHeaders != null && !authHeaders.isEmpty()) {
            authorizationHeader = authHeaders.getFirst();
        } else {
            authorizationHeader = bearerFromCookie(request);
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

    private static String bearerFromCookie(WebGraphQlRequest request) {
        var cookieHeaders = request.getHeaders().get("Cookie");
        if (cookieHeaders == null) {
            return null;
        }
        for (String header : cookieHeaders) {
            for (String part : header.split(";")) {
                String trimmed = part.trim();
                if (trimmed.startsWith(ACCESS_TOKEN_COOKIE_PREFIX)) {
                    String token = trimmed.substring(ACCESS_TOKEN_COOKIE_PREFIX.length());
                    if (!token.isEmpty()) {
                        return "Bearer " + token;
                    }
                }
            }
        }
        return null;
    }
}

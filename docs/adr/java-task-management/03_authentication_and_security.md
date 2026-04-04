# ADR 03: Authentication and Security

## Overview

This document explains how the Java Task Management system authenticates users and hardens itself against common web security threats. We cover JWT access tokens with refresh token rotation, Google OAuth2 login, Spring Security filter chains, CORS whitelisting, security headers, and the inter-service trust model where the gateway validates JWTs and forwards a trusted `X-User-Id` header to downstream services.

If you have built auth middleware in Go or Express, the concepts here will feel familiar -- the difference is that Spring Security gives you a declarative, filter-chain-based framework instead of writing raw middleware functions. This document explains *why* we chose each approach and how the pieces fit together.

---

## Architecture Context

```
Browser
  |
  |  POST /api/auth/google  { code, redirectUri }
  v
[task-service]  -->  Google OAuth2 token endpoint  -->  Google userinfo endpoint
  |
  |  Returns { accessToken, refreshToken, userId, email, name, avatarUrl }
  v
Browser stores tokens
  |
  |  GET /api/tasks  Authorization: Bearer <accessToken>
  v
[gateway-service]
  |  JwtAuthenticationFilter validates token
  |  Extracts userId, forwards X-User-Id header
  v
[task-service / activity-service / notification-service]
  |  Trusts X-User-Id from gateway (no re-validation)
```

The key insight: **only the gateway and task-service (for auth endpoints) need to validate JWTs**. Downstream services behind the gateway receive a pre-validated `X-User-Id` header, keeping them simpler.

---

## Package Introductions

### jjwt (io.jsonwebtoken)

We use the [jjwt](https://github.com/jwtk/jjwt) library for JWT creation and validation.

**Why jjwt over alternatives?**

| Library | Pros | Cons | Verdict |
|---------|------|------|---------|
| **jjwt** | Fluent builder API, actively maintained, handles key sizing automatically, excellent docs | Extra dependency | **Chosen** -- best DX for HMAC-signed JWTs |
| Spring Security OAuth2 Resource Server | Built into Spring, supports JWK sets | Designed for external IdP (Keycloak, Auth0), overkill for self-issued JWTs | Too much ceremony for our use case |
| nimbus-jose-jwt | Comprehensive JOSE support, used internally by Spring | Lower-level API, verbose for simple HMAC signing | Better for JWK/JWE scenarios |
| jose4j | Full JOSE spec coverage | Less popular, API is more complex | No compelling advantage |

jjwt's builder pattern makes token creation a single readable chain -- compare line 32-38 in JwtService to what you would write with nimbus-jose, and the difference is stark.

### Spring Security

Spring Security provides the filter chain that intercepts every HTTP request, decides whether it needs authentication, and either allows or rejects it. Think of it as the framework equivalent of writing `app.use(authMiddleware)` in Express -- except it is declarative and composable.

### Spring Web (RestClient)

The `AuthController` uses Spring's `RestClient` (introduced in Spring 6.1) to call Google's OAuth2 endpoints. This is the modern replacement for `RestTemplate`, with a fluent API similar to what you would expect from `fetch` or Go's `http.Client`.

---

## Go/TS Comparison

| Concept | Go | TypeScript (Express) | Java (Spring Boot) |
|---------|-----|---------------------|---------------------|
| JWT signing | `golang-jwt/jwt/v5` -- manual claims struct + `SignedString()` | `jsonwebtoken` npm -- `jwt.sign(payload, secret)` | jjwt -- `Jwts.builder().subject().signWith().compact()` |
| JWT validation | `jwt.Parse(token, keyFunc)` with custom `keyFunc` | `jwt.verify(token, secret)` | `Jwts.parser().verifyWith(key).build().parseSignedClaims()` |
| Auth middleware | `func AuthMiddleware(next http.Handler) http.Handler` -- explicit wrapping | `passport.authenticate('jwt')` or manual middleware | `OncePerRequestFilter` subclass added to the SecurityFilterChain |
| Route protection | `r.Use(authMiddleware)` on router group | `app.use('/api', authMiddleware)` | `.authorizeHttpRequests(auth -> auth.requestMatchers(...).authenticated())` |
| CORS | `rs/cors` middleware | `cors` npm package | `CorsConfigurationSource` bean |
| Session management | Stateless by default (no framework sessions) | `express-session` (opt-in) | `SessionCreationPolicy.STATELESS` (explicit opt-out of Spring's default) |
| Error handling | Return `error` from handler, middleware catches | `next(err)` to error middleware | `@RestControllerAdvice` with `@ExceptionHandler` methods |

**Key difference for Go developers:** In Go, you compose middleware manually -- `authMiddleware(rateLimiter(handler))`. In Spring, you declare a `SecurityFilterChain` bean and Spring wires the filter ordering for you. The chain is: CORS filter -> your JWT filter -> authorization check -> your controller.

---

## Build It

### Step 1: JwtService -- A Plain Class, Not a @Service

The first design decision: `JwtService` is a **plain Java class** with no Spring annotations. It is registered as a `@Bean` through `JwtConfig`:

```java
// JwtConfig.java
@Configuration
public class JwtConfig {

    @Bean
    public JwtService jwtService(
            @Value("${app.jwt.secret}") String secret,
            @Value("${app.jwt.access-token-ttl-ms:900000}") long accessTtl,
            @Value("${app.jwt.refresh-token-ttl-ms:604800000}") long refreshTtl) {
        return new JwtService(secret, accessTtl, refreshTtl);
    }
}
```

**Why not `@Service` directly on JwtService?**

If `JwtService` were annotated with `@Service`, Spring would manage its construction and you would need `@Value` annotations on the constructor parameters or fields. That couples the class to Spring's DI container. By keeping it a plain class:

1. **Unit tests are trivial** -- `new JwtService("test-secret-thats-long-enough", 900000, 604800000)` with no Spring context needed.
2. **Constructor validates invariants** -- all required values are constructor parameters, so you cannot create a half-initialized instance.
3. **Reusable** -- if you ever need JwtService in a non-Spring context (CLI tool, test harness), it works without modification.

This is the same pattern you would use in Go: define a struct with constructor function, then wire it in `main()`. The `@Bean` method in `JwtConfig` is the Java equivalent of Go's `func NewJwtService(secret string, ttl time.Duration) *JwtService`.

### Step 2: Token Generation and Validation

```java
// JwtService.java -- generating an access token
public String generateAccessToken(UUID userId, String email) {
    Date now = new Date();
    Date expiry = new Date(now.getTime() + accessTokenTtlMs);

    return Jwts.builder()
            .subject(userId.toString())         // standard JWT "sub" claim
            .claim("email", email)              // custom claim
            .issuedAt(now)                      // "iat" claim
            .expiration(expiry)                 // "exp" claim
            .signWith(signingKey)               // HMAC-SHA256 (key size determines algo)
            .compact();                         // serialize to compact JWT string
}
```

The `signingKey` is created from the secret string using `Keys.hmacShaKeyFor()`, which automatically selects HS256, HS384, or HS512 based on key length. This is a safety feature -- jjwt refuses to use a key that is too short for the algorithm.

**Refresh tokens are opaque** -- just a random UUID string stored in the database. They are not JWTs because:
- Refresh tokens need to be revocable (delete the DB row).
- They do not carry claims that need to be verified client-side.
- Simpler is better when the token never leaves the server-to-server exchange.

```java
public String generateRefreshTokenString() {
    return UUID.randomUUID().toString();
}
```

**Validation** uses a try-catch around `parseClaims()`. If the signature is invalid, the token is expired, or the token is malformed, jjwt throws an exception and `isValid()` returns false:

```java
public boolean isValid(String token) {
    try {
        parseClaims(token);
        return true;
    } catch (Exception e) {
        return false;
    }
}
```

### Step 3: Google OAuth2 Authorization Code Flow

The `AuthController` implements the server-side of the OAuth2 authorization code flow. Here is what happens step by step:

1. **Frontend** redirects user to Google's consent screen with `client_id`, `redirect_uri`, and `scope`.
2. **Google** redirects back to the frontend with an **authorization code**.
3. **Frontend** sends the code to `POST /api/auth/google`.
4. **Backend** exchanges the code for a Google access token at `https://oauth2.googleapis.com/token`.
5. **Backend** uses that Google access token to fetch the user's profile from `https://www.googleapis.com/oauth2/v3/userinfo`.
6. **Backend** creates or updates the user in the database and issues our own JWT + refresh token.

```java
// AuthController.java -- the code exchange
@PostMapping("/google")
public AuthResponse googleLogin(@Valid @RequestBody AuthRequest request) {
    // Step 4: Exchange authorization code for Google tokens
    MultiValueMap<String, String> tokenParams = new LinkedMultiValueMap<>();
    tokenParams.add("code", request.code());
    tokenParams.add("client_id", googleClientId);
    tokenParams.add("client_secret", googleClientSecret);
    tokenParams.add("redirect_uri", request.redirectUri());
    tokenParams.add("grant_type", "authorization_code");

    Map<String, Object> tokenResponse = restClient.post()
            .uri(googleTokenUrl)
            .contentType(MediaType.APPLICATION_FORM_URLENCODED)
            .body(tokenParams)
            .retrieve()
            .body(Map.class);

    String accessTokenGoogle = (String) tokenResponse.get("access_token");

    // Step 5: Fetch user profile
    Map<String, Object> userInfo = restClient.get()
            .uri(googleUserInfoUrl)
            .header("Authorization", "Bearer " + accessTokenGoogle)
            .retrieve()
            .body(Map.class);

    // Step 6: Create/update user, issue our tokens
    return authService.authenticateGoogleUser(
            (String) userInfo.get("email"),
            (String) userInfo.get("name"),
            (String) userInfo.get("picture"));
}
```

**Why handle this server-side instead of using Spring's OAuth2 Client?** Spring's OAuth2 Client is designed for server-rendered apps where Spring itself manages the redirect flow. Our frontend is a React SPA that handles the redirect and sends the code. The backend only needs to do the token exchange -- which is a single HTTP call, not worth pulling in the full OAuth2 Client autoconfiguration.

### Step 4: AuthService -- User Upsert and Token Issuance

```java
// AuthService.java
@Transactional
public AuthResponse authenticateGoogleUser(String email, String name, String avatarUrl) {
    User user = userRepository.findByEmail(email).orElse(null);

    if (user == null) {
        user = new User(email, name, avatarUrl);
        user = userRepository.save(user);
    } else {
        user.setName(name);
        user.setAvatarUrl(avatarUrl);
    }

    return issueTokens(user);
}
```

This is a classic "find or create" pattern. On first login, a new User row is created. On subsequent logins, the name and avatar are updated (Google profile changes). The `@Transactional` annotation ensures the user save and refresh token save happen atomically.

The `issueTokens` method generates both tokens and persists the refresh token:

```java
private AuthResponse issueTokens(User user) {
    String accessToken = jwtService.generateAccessToken(user.getId(), user.getEmail());
    String refreshTokenStr = jwtService.generateRefreshTokenString();
    Instant expiresAt = Instant.now().plusMillis(jwtService.getRefreshTokenTtlMs());

    RefreshToken refreshToken = new RefreshToken(user, refreshTokenStr, expiresAt);
    refreshTokenRepository.save(refreshToken);

    return new AuthResponse(accessToken, refreshTokenStr,
            user.getId(), user.getEmail(), user.getName(), user.getAvatarUrl());
}
```

### Step 5: JwtAuthenticationFilter -- The Request Gatekeeper

This is the core of the auth system. Every request passes through this filter:

```java
// JwtAuthenticationFilter.java
public class JwtAuthenticationFilter extends OncePerRequestFilter {

    private final JwtService jwtService;

    @Override
    protected void doFilterInternal(HttpServletRequest request,
                                    HttpServletResponse response,
                                    FilterChain filterChain)
            throws ServletException, IOException {

        String authHeader = request.getHeader("Authorization");

        if (authHeader != null && authHeader.startsWith("Bearer ")) {
            String token = authHeader.substring(7);
            if (jwtService.isValid(token)) {
                UUID userId = jwtService.extractUserId(token);
                String email = jwtService.extractEmail(token);

                UsernamePasswordAuthenticationToken authentication =
                        new UsernamePasswordAuthenticationToken(
                                userId.toString(), null, Collections.emptyList());
                authentication.setDetails(
                        new WebAuthenticationDetailsSource().buildDetails(request));
                SecurityContextHolder.getContext().setAuthentication(authentication);
            }
        }

        filterChain.doFilter(request, response);
    }
}
```

**Why `OncePerRequestFilter`?** In Spring's servlet architecture, a request can pass through a filter multiple times (e.g., on forwards or includes). `OncePerRequestFilter` guarantees the filter logic runs exactly once per request, preventing double-validation.

**Why `UsernamePasswordAuthenticationToken` even though we do not have a password?** This is Spring Security's general-purpose authentication token. The name is misleading -- it is really just a container for (principal, credentials, authorities). We set principal to the userId string, credentials to null, and authorities to an empty list.

**The flow:**
1. Extract `Authorization` header.
2. If it starts with `Bearer `, extract the token string.
3. Validate the token (signature + expiry).
4. If valid, set the `SecurityContext` -- downstream code (controllers, services) can now call `SecurityContextHolder.getContext().getAuthentication().getName()` to get the userId.
5. Always call `filterChain.doFilter()` -- even if the token is missing or invalid. The *authorization* layer (configured in SecurityConfig) handles rejection.

**Go comparison:** This is equivalent to a Go middleware that checks the `Authorization` header, validates the JWT, and puts the user ID into `context.Context`. The difference is that Spring uses a thread-local `SecurityContextHolder` instead of request-scoped context.

### Step 6: SecurityConfig -- Wiring It All Together

```java
// SecurityConfig.java (task-service)
@Configuration
@EnableWebSecurity
public class SecurityConfig {

    @Value("${app.allowed-origins:http://localhost:3000}")
    private String allowedOrigins;

    @Bean
    public SecurityFilterChain securityFilterChain(HttpSecurity http,
                                                    JwtService jwtService) throws Exception {
        JwtAuthenticationFilter jwtFilter = new JwtAuthenticationFilter(jwtService);

        return http
                .csrf(csrf -> csrf.disable())
                .cors(cors -> cors.configurationSource(corsConfigurationSource()))
                .sessionManagement(session ->
                        session.sessionCreationPolicy(SessionCreationPolicy.STATELESS))
                .headers(headers -> headers
                        .contentTypeOptions(contentType -> {})
                        .frameOptions(frame -> frame.deny())
                        .httpStrictTransportSecurity(hsts -> hsts
                                .includeSubDomains(true)
                                .maxAgeInSeconds(31536000)))
                .authorizeHttpRequests(auth -> auth
                        .requestMatchers("/api/auth/**").permitAll()
                        .requestMatchers("/actuator/health").permitAll()
                        .anyRequest().authenticated())
                .addFilterBefore(jwtFilter, UsernamePasswordAuthenticationFilter.class)
                .build();
    }
}
```

Breaking down each decision:

- **`.csrf(csrf -> csrf.disable())`** -- CSRF protection is for cookie-based sessions. Our API uses Bearer tokens, so CSRF is not applicable. Leaving it enabled would block legitimate API calls.
- **`SessionCreationPolicy.STATELESS`** -- Tells Spring not to create HTTP sessions. Every request must carry its own credentials (the JWT). This is critical for horizontal scaling -- no session store needed.
- **`.requestMatchers("/api/auth/**").permitAll()`** -- Auth endpoints must be accessible without a token (you need to log in to get a token).
- **`.requestMatchers("/actuator/health").permitAll()`** -- Health checks for Docker/load balancers need to work without auth.
- **`.anyRequest().authenticated()`** -- Everything else requires a valid JWT.
- **`.addFilterBefore(jwtFilter, UsernamePasswordAuthenticationFilter.class)`** -- Insert our JWT filter before Spring's default username/password filter.

### Step 7: Why We Removed the X-User-Id Fallback

An earlier version of the task-service had a fallback in the filter: if no JWT was present, check for an `X-User-Id` header and trust it. This was intended for the inter-service trust model where the gateway forwards the user ID.

**This was an auth bypass vulnerability.** Any client could skip authentication entirely by sending `X-User-Id: <any-uuid>` directly to the task-service. We removed it and instead:

1. The gateway validates the JWT and sets `X-User-Id`.
2. Downstream services (behind the gateway) read `X-User-Id` from the request, trusting that the gateway already validated it.
3. The task-service (which also has public auth endpoints) uses JWT validation only -- no header fallback.

This is the same trust boundary pattern used in Go microservices: the edge service (gateway) does the heavy auth work, and internal services trust headers from the gateway.

### Step 8: Security Headers

The `headers` configuration in SecurityConfig adds three important HTTP response headers:

```java
.headers(headers -> headers
        .contentTypeOptions(contentType -> {})    // X-Content-Type-Options: nosniff
        .frameOptions(frame -> frame.deny())       // X-Frame-Options: DENY
        .httpStrictTransportSecurity(hsts -> hsts  // Strict-Transport-Security
                .includeSubDomains(true)
                .maxAgeInSeconds(31536000)))
```

| Header | Value | Purpose |
|--------|-------|---------|
| `X-Content-Type-Options` | `nosniff` | Prevents browsers from MIME-type sniffing responses, blocking attacks where a malicious file is served with a wrong Content-Type |
| `X-Frame-Options` | `DENY` | Prevents the page from being embedded in an iframe, blocking clickjacking attacks |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | Forces HTTPS for one year, including subdomains |

### Step 9: CORS Configuration

```java
@Bean
public CorsConfigurationSource corsConfigurationSource() {
    CorsConfiguration config = new CorsConfiguration();
    List<String> origins = Arrays.asList(allowedOrigins.split(","));
    config.setAllowedOrigins(origins);
    config.setAllowedMethods(List.of("GET", "POST", "PUT", "DELETE", "OPTIONS"));
    config.setAllowedHeaders(List.of("Authorization", "Content-Type", "X-Requested-With"));
    config.setAllowCredentials(true);

    UrlBasedCorsConfigurationSource source = new UrlBasedCorsConfigurationSource();
    source.registerCorsConfiguration("/**", config);
    return source;
}
```

The allowed origins come from an environment variable (`app.allowed-origins`), defaulting to `http://localhost:3000`. In production, this is set to the actual frontend domain. **Never use `*` with `allowCredentials(true)`** -- browsers reject it, and even if they did not, it would defeat the purpose of CORS entirely.

### Step 10: GlobalExceptionHandler -- No Stack Traces in Responses

```java
@RestControllerAdvice
public class GlobalExceptionHandler {

    @ExceptionHandler(IllegalArgumentException.class)
    @ResponseStatus(HttpStatus.BAD_REQUEST)
    public Map<String, String> handleBadRequest(IllegalArgumentException ex) {
        return Map.of("error", ex.getMessage());
    }

    @ExceptionHandler(Exception.class)
    @ResponseStatus(HttpStatus.INTERNAL_SERVER_ERROR)
    public Map<String, String> handleGeneral(Exception ex) {
        return Map.of("error", "Internal server error");
    }
}
```

The catch-all handler returns a generic "Internal server error" message. **Never return `ex.getMessage()` for unexpected exceptions** -- it may contain SQL queries, file paths, or other internal details that help attackers.

### Step 11: Gateway vs Task-Service JwtService -- Same Logic, Different Wiring

Compare the two JwtService classes:

**Gateway JwtService** -- uses `@Service` annotation, only needs validation (no token generation):
```java
@Service
public class JwtService {
    // Only extractUserId() and isValid() -- no generate methods
}
```

**Task-service JwtService** -- plain class configured via `@Bean`, has generation + validation:
```java
public class JwtService {
    // generateAccessToken(), generateRefreshTokenString(), extractUserId(), isValid()
}
```

The gateway only validates tokens and extracts the user ID -- it never creates them. The task-service does both because it hosts the auth endpoints. The gateway uses `@Service` because it is simpler (only one constructor parameter) and does not need the same testability flexibility.

---

## Experiment

Try these changes to deepen your understanding:

1. **Shorten the access token TTL** to 30 seconds (`app.jwt.access-token-ttl-ms=30000`). Watch the token expire quickly and observe how the refresh flow kicks in.

2. **Add an `issuer` claim** to JwtService: `.issuer("task-management")` in the builder, and `.requireIssuer("task-management")` in the parser. This prevents tokens from one system being accepted by another.

3. **Add role-based authorization.** Add a `role` claim to the JWT, extract it in the filter, and create `SimpleGrantedAuthority` objects. Then use `.requestMatchers("/api/admin/**").hasRole("ADMIN")` in SecurityConfig.

4. **Remove the CORS config** entirely and test from the frontend. Observe the browser's error message in the console -- this is what CORS is protecting against.

5. **Try sending an `X-User-Id` header directly** to a protected endpoint without a JWT. Verify it gets a 401 -- confirming the old auth bypass vulnerability is gone.

---

## Check Your Understanding

1. Why is the `JwtAuthenticationFilter` a plain class in task-service but `@Component` in gateway-service? What are the tradeoffs of each approach?

2. If you removed `SessionCreationPolicy.STATELESS`, what would happen? Would auth still work? What subtle bugs could appear?

3. The `refreshAccessToken` method creates a new refresh token on every refresh call. Why is this more secure than reusing the same refresh token? (Hint: think about token theft detection.)

4. Why does the task-service handle Google OAuth directly instead of delegating to the gateway? What would change if you moved auth to the gateway?

5. In the `doFilterInternal` method, we always call `filterChain.doFilter()` even when the token is invalid. Why not return a 401 immediately? What role does the authorization layer play?

6. Compare this auth architecture to how you would build it in Go with `chi` or `gin`. What would be easier? What would be harder?

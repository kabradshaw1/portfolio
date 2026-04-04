# ADR 06 — GraphQL Gateway Pattern with Spring for GraphQL

## Overview

The gateway-service is the single entry point for the frontend. Instead of exposing the REST APIs of the task-service, activity-service, and notification-service directly to the client, it presents a unified GraphQL API. The client sends one request describing exactly what data it needs, and the gateway aggregates responses from multiple downstream services.

This document walks through why GraphQL was chosen over a simpler REST proxy, how Spring for GraphQL's schema-first approach works, how HTTP context (user identity) flows from the incoming request through the GraphQL execution into downstream service calls, and why `RestClient` was chosen for calling downstream services over the alternatives.

By the end, you will understand the full request lifecycle: HTTP request → JWT → `GraphQlInterceptor` → GraphQL context → `@QueryMapping` / `@MutationMapping` → service client → downstream REST call → response assembled by the GraphQL runtime.

---

## Architecture Context

```
Frontend (Next.js)
      |
      | POST /graphql  { query, variables }
      |
gateway-service :8080
      |
      +--- GraphQlInterceptor         (copies userId from SecurityContext → GraphQL context)
      |
      +--- QueryResolver              (handles all Query {} fields)
      +--- MutationResolver           (handles all Mutation {} fields)
      |
      +--- TaskServiceClient    ---> task-service :8081     (REST)
      +--- ActivityServiceClient ---> activity-service :8082 (REST)
      +--- NotificationServiceClient -> notification-service :8083 (REST)
```

The gateway does **not** own any database. It is a pure aggregation and translation layer. All persistence is in the downstream services.

**Why this topology?**

The frontend has a single GraphQL endpoint to configure, authenticate against, and reason about. The downstream microservices stay simple — they speak plain REST and don't know about GraphQL. The gateway absorbs the complexity of protocol translation and data assembly.

**What Spring for GraphQL does NOT do automatically:** It does not federate schemas, does not handle N+1 queries via DataLoader by default, and does not batch requests across services. Those are advanced topics. This gateway is deliberately simple — one-to-one mapping between GraphQL resolvers and downstream REST calls.

---

## Why GraphQL BFF and Not Spring Cloud Gateway

This is the most important architectural decision in this service. There are two common patterns for building a gateway:

**Spring Cloud Gateway** is a reverse proxy and routing engine. It forwards HTTP requests to downstream services based on URL patterns. It knows nothing about the shape of the data. It is the right choice when your frontend speaks the same protocol as your backends (REST → REST) and you primarily need load balancing, rate limiting, circuit breaking, and authentication at the edge.

**GraphQL BFF (Backend For Frontend)** is an aggregation layer. It speaks GraphQL to the frontend and REST (or gRPC, or anything) to backends. It is the right choice when:
- The frontend needs to combine data from multiple services in a single request
- Different clients (web, mobile) need different data shapes from the same underlying services
- You want to evolve the frontend API contract independently of backend service APIs
- You need to hide the internal service topology from the frontend

This project uses the GraphQL BFF because the task detail page needs tasks (from task-service), activity events (from activity-service), comments (from activity-service), and notification counts (from notification-service) all at once. Without the BFF, the frontend would make four separate REST calls and assemble the data itself. With the BFF, it makes one GraphQL query.

---

## Package Introductions

### Spring for GraphQL

**What it is:** The official Spring project for building GraphQL servers in Spring Boot. It is schema-first: you write a `.graphqls` schema file, and Spring maps GraphQL field resolution to annotated Java methods.

**What you get:**
- Schema loading from `classpath:graphql/` (configured in `application.yml`)
- `@QueryMapping` / `@MutationMapping` — map methods to schema fields
- `@Argument` — bind GraphQL input arguments to method parameters
- `DataFetchingEnvironment` — access GraphQL context, field selection, etc.
- `WebGraphQlInterceptor` — intercept requests before schema execution (used here for auth context)
- Built-in `/graphql` HTTP endpoint
- Optional GraphiQL browser UI at `/graphiql`

**Maven dependency:**
```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-graphql</artifactId>
</dependency>
```

**Configuration (application.yml):**
```yaml
spring:
  graphql:
    graphiql:
      enabled: ${GRAPHIQL_ENABLED:false}   # enable in dev, disable in prod
    schema:
      locations: classpath:graphql/         # looks for *.graphqls files here
```

**Alternatives considered:**

| Option | Why not chosen |
|---|---|
| `graphql-java` directly | Low-level. You wire up everything manually — type registries, data fetchers, schema wiring. Spring for GraphQL wraps this with convention-over-configuration. |
| Netflix DGS Framework | Facebook/Netflix-authored Spring Boot extension. Heavier, more opinionated, excellent for large teams. Overkill for this project's scale. |
| `graphql-java-tools` / graphql-kotlin | graphql-java-tools is unmaintained. graphql-kotlin is code-first only. |
| Spring Cloud Gateway (REST proxy) | Routes requests, doesn't aggregate. Cannot serve the frontend's need to combine data from multiple services per request. |

---

### RestClient (Spring 6.1+)

**What it is:** A synchronous, fluent HTTP client introduced in Spring Framework 6.1 (Spring Boot 3.2+). It is the successor to `RestTemplate` with a modern builder API, and the synchronous alternative to `WebClient`.

**Why not WebClient?**
`WebClient` is reactive — it returns `Mono<T>` and `Flux<T>` and requires thinking in reactive streams (subscribers, operators, backpressure). It is the right choice when you need non-blocking I/O across the entire stack. This gateway uses Spring MVC (servlet stack), not Spring WebFlux. Using `WebClient` in a servlet application works but creates a conceptual mismatch — you end up calling `.block()` to extract the value synchronously, which defeats the purpose of reactive. `RestClient` is the idiomatic choice for synchronous servlet applications in Spring Boot 3.2+.

**Why not Feign (OpenFeign)?**
Feign is a declarative HTTP client — you define an interface annotated with `@FeignClient`, `@GetMapping`, etc., and Spring generates the implementation. It is elegant for service-to-service calls. The reason `RestClient` was chosen here is pedagogical: `RestClient`'s fluent API is explicit and easy to read without knowing Feign's annotation model. For a learning project, seeing the HTTP call constructed line by line is more instructive.

**Maven dependency:**
```xml
<!-- Included in spring-boot-starter-web, no additional dependency needed -->
```

**Alternatives considered:**

| Option | Why not chosen |
|---|---|
| `RestTemplate` | Deprecated for new code in Spring 6. `RestClient` is the replacement. |
| `WebClient` | Reactive. Requires `.block()` in a servlet context. Conceptual mismatch. |
| OpenFeign | Declarative, elegant, but hides the HTTP calls behind annotations. Less transparent for learning. |
| Apache HttpClient / OkHttp | Lower-level. Would need manual Jackson integration. No Spring integration. |

---

### Schema-First vs. Code-First GraphQL

Two philosophies exist for building GraphQL APIs:

**Schema-first:** You write the `.graphqls` schema file, then implement resolvers for it. The schema is the contract. If the schema and the Java code disagree, startup fails. This project uses schema-first.

**Code-first:** You annotate Java classes and methods, and a library generates the schema from those annotations. The code is the source of truth. The schema is generated, not hand-written.

**Why schema-first here:**
- The schema file is readable by non-Java developers (frontend engineers, designers)
- It enforces thinking about the API contract before the implementation
- Changes to the schema are visible in a single `.graphqls` file during code review
- Spring for GraphQL's schema-first support is mature and well-documented

In Go, the dominant GraphQL library `gqlgen` is schema-first. In TypeScript, both approaches are common: `Apollo Server` with `typeDefs` + `resolvers` is schema-first; `TypeGraphQL` is code-first.

---

## Go / TypeScript Comparison

| Concept | Go (gqlgen) | TypeScript (Apollo Server) | Java (Spring for GraphQL) |
|---|---|---|---|
| Schema definition | `.graphqls` file (schema-first) | `typeDefs` string/SDL or codegen | `.graphqls` file in `classpath:graphql/` |
| Resolver binding | Generated `ResolverRoot` interface, implement in Go | `resolvers` object with `Query` and `Mutation` maps | `@Controller` class with `@QueryMapping` / `@MutationMapping` methods |
| Input argument binding | Generated input structs, passed directly | Typed arguments from resolver function params | `@Argument` annotation on method parameters, or `Map<String, Object>` for input types |
| Request context | `context.Context` passed through resolver chain | `context` object in resolver function | `DataFetchingEnvironment.getGraphQlContext()` |
| Auth context injection | Middleware adds user to `context.WithValue` | ApolloServer `context` factory function | `WebGraphQlInterceptor.intercept()` modifies `GraphQLContext` |
| HTTP client for downstream | `net/http` or `resty` | `axios`, `node-fetch`, `got` | `RestClient` (Spring 6.1+) |
| HTTP client config | Manual base URL, default headers | Axios instance with `baseURL` | `RestClient.builder().baseUrl(url).build()` injected as Spring bean |
| Generic list response type | `[]ProjectDto` | `ProjectDto[]` | `List<ProjectDto>`, needs `ParameterizedTypeReference<List<ProjectDto>>` |
| Startup schema validation | gqlgen generates code — mismatches are compile errors | Runtime if using codegen, otherwise silent | Spring validates schema vs. resolvers at startup — fails fast |
| Code-first alternative | Not idiomatic in gqlgen | `TypeGraphQL`, `Nexus`, `Pothos` | Netflix DGS with `@DgsComponent` |

**The biggest conceptual shift from Go/gqlgen:** In Go, `gqlgen` generates code. You run `go generate`, and it creates the `ResolverRoot` interface that your types must implement. Type safety is enforced at compile time — if you add a field to the schema and don't implement the resolver, it won't compile. In Spring for GraphQL, the binding is by convention (method name matches field name). If you add `newField: String` to the schema and don't add a `@QueryMapping public String newField()` method, the server starts but returns null for that field — no compile error.

**The biggest conceptual shift from TypeScript/Apollo:** Apollo Server resolvers receive `(parent, args, context, info)` as positional parameters every time. Spring uses `@Argument` for specific args and `DataFetchingEnvironment` as an injectable parameter when you need context or the full environment. The mapping is more explicit — you annotate exactly what you want rather than destructuring a fixed positional signature.

---

## Build It

### Step 1 — The Schema

```graphql
# gateway-service/src/main/resources/graphql/schema.graphqls

type Query {
    me: User
    project(id: ID!): Project
    myProjects: [Project!]!
    task(id: ID!): Task
    taskActivity(taskId: ID!): [ActivityEvent!]!
    taskComments(taskId: ID!): [Comment!]!
    myNotifications(unreadOnly: Boolean): NotificationResponse!
}

type Mutation {
    createProject(input: CreateProjectInput!): Project!
    updateProject(id: ID!, input: UpdateProjectInput!): Project!
    deleteProject(id: ID!): Boolean!
    createTask(input: CreateTaskInput!): Task!
    updateTask(id: ID!, input: UpdateTaskInput!): Task!
    deleteTask(id: ID!): Boolean!
    assignTask(taskId: ID!, userId: ID!): Task!
    addComment(taskId: ID!, body: String!): Comment!
    markNotificationRead(id: ID!): Boolean!
    markAllNotificationsRead: Boolean!
}
```

**Reading this schema:**

- `ID!` — non-null GraphQL ID scalar (serialized as a string, semantically an identifier)
- `String` (no `!`) — nullable string
- `String!` — non-null string
- `[Project!]!` — non-null list of non-null Projects (the list cannot be null, and no element in it can be null)
- `Boolean` without `!` would mean the boolean itself can be null — unusual but valid in GraphQL

**Types and inputs:**
```graphql
type Task {
    id: ID!, projectId: ID!, title: String!, description: String,
    status: TaskStatus!, priority: TaskPriority!, assigneeId: ID, assigneeName: String,
    dueDate: String, createdAt: String!, updatedAt: String!
}

enum TaskStatus { TODO, IN_PROGRESS, DONE }
enum TaskPriority { LOW, MEDIUM, HIGH }

input CreateTaskInput {
    projectId: ID!, title: String!, description: String,
    priority: TaskPriority, dueDate: String
}
```

The distinction between `type` and `input` is fundamental to GraphQL:
- `type` — output type, returned from queries and mutations
- `input` — input type, used as mutation arguments, cannot reference output types

Enums in GraphQL schema map to Java enums by name in the DTO classes. Spring for GraphQL handles the conversion automatically.

Notice that `ActivityEvent.metadata` in the schema is typed as `String!` (serialized JSON string), not a nested object. This is a deliberate simplification — the gateway receives `Map<String, Object>` from the activity-service and serializes it to a string for GraphQL consumers. A more sophisticated schema would define a custom scalar or a union type.

---

### Step 2 — RestClientConfig

```java
// gateway-service/config/RestClientConfig.java

@Configuration
public class RestClientConfig {

    @Bean("taskRestClient")                                       // (1)
    public RestClient taskServiceClient(
            @Value("${app.services.task-url}") String taskUrl) {  // (2)
        return RestClient.builder()
                .baseUrl(taskUrl)                                  // (3)
                .build();
    }

    @Bean("activityRestClient")
    public RestClient activityServiceClient(
            @Value("${app.services.activity-url}") String activityUrl) {
        return RestClient.builder()
                .baseUrl(activityUrl)
                .build();
    }

    @Bean("notificationRestClient")
    public RestClient notificationServiceClient(
            @Value("${app.services.notification-url}") String notificationUrl) {
        return RestClient.builder()
                .baseUrl(notificationUrl)
                .build();
    }
}
```

**(1) `@Bean("taskRestClient")`**
Creates a named Spring bean. The name is important because there are three `RestClient` beans in the same application context. Without the qualifier name, Spring wouldn't know which `RestClient` to inject where.

**(2) `@Value("${app.services.task-url}")`**
Reads from `application.yml`:
```yaml
app:
  services:
    task-url: ${TASK_SERVICE_URL:http://localhost:8081}
```
The `${TASK_SERVICE_URL:http://localhost:8081}` syntax means: use the environment variable `TASK_SERVICE_URL` if it exists, otherwise default to `http://localhost:8081`. This is the standard Spring Boot 12-factor config pattern.

**(3) `baseUrl(taskUrl)`**
All requests made through this client are relative to the base URL. When `TaskServiceClient` calls `.uri("/projects")`, it resolves to `http://localhost:8081/projects`. This is equivalent to:
- Go: `client := resty.New().SetBaseURL(taskUrl)`
- TypeScript: `const client = axios.create({ baseURL: taskUrl })`

---

### Step 3 — TaskServiceClient

```java
// gateway-service/client/TaskServiceClient.java

@Component
public class TaskServiceClient {

    private final RestClient client;

    public TaskServiceClient(@Qualifier("taskRestClient") RestClient client) {  // (1)
        this.client = client;
    }

    public List<ProjectDto> getMyProjects(String userId) {
        return client.get()
                .uri("/projects")
                .header("X-User-Id", userId)                                     // (2)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});                    // (3)
    }

    public ProjectDto getProject(String id) {
        return client.get()
                .uri("/projects/{id}", id)                                       // (4)
                .retrieve()
                .body(ProjectDto.class);
    }

    public ProjectDto createProject(String userId, Map<String, Object> input) {
        return client.post()
                .uri("/projects")
                .header("X-User-Id", userId)
                .body(input)                                                      // (5)
                .retrieve()
                .body(ProjectDto.class);
    }

    public TaskDto assignTask(String taskId, String assigneeId, String userId) {
        return client.put()
                .uri("/tasks/{taskId}/assign/{assigneeId}", taskId, assigneeId)  // (6)
                .header("X-User-Id", userId)
                .retrieve()
                .body(TaskDto.class);
    }

    public void deleteProject(String id, String userId) {
        client.delete()
                .uri("/projects/{id}", id)
                .header("X-User-Id", userId)
                .retrieve()
                .toBodilessEntity();                                              // (7)
    }
}
```

**(1) `@Qualifier("taskRestClient")`**
Tells Spring: "inject the bean named `taskRestClient`." Without this, Spring finds three `RestClient` beans and throws `NoUniqueBeanDefinitionException`. This is the matching counterpart to `@Bean("taskRestClient")` in `RestClientConfig`.

**(2) `.header("X-User-Id", userId)`**
The downstream services authenticate requests via the `X-User-Id` header — they trust that the gateway has already verified the JWT and extracted the user ID. This is the "internal trust" pattern for microservices. The gateway is the authentication boundary; internal services trust gateway-forwarded headers. In a more hardened setup, you'd use mutual TLS or a service mesh instead.

**(3) `new ParameterizedTypeReference<>() {}`**
This is Java's answer to a fundamental type erasure problem. At runtime, Java generics are erased — `List<ProjectDto>` becomes just `List`. `RestClient` needs to know the full generic type to deserialize the JSON array correctly. `ParameterizedTypeReference` captures the generic type at compile time and carries it through to the deserializer. The empty `{}` creates an anonymous subclass — it's the standard Java idiom. In Go: `var projects []ProjectDto; json.Unmarshal(body, &projects)`. In TypeScript: `axios.get<ProjectDto[]>(url)` with `response.data`.

**(4) `.uri("/projects/{id}", id)`**
URI template variables. The `{id}` is replaced with the `id` argument at call time. RestClient URL-encodes the variable automatically. Equivalent to:
- Go: `fmt.Sprintf("/projects/%s", id)` (no encoding)
- TypeScript: `axios.get(\`/projects/${id}\`)` (no encoding)

**(5) `.body(input)` where `input` is `Map<String, Object>`**
RestClient serializes the map to JSON using Jackson. The downstream service receives a JSON object with the same keys. This works because `Map<String, Object>` is already a JSON-compatible structure — Spring's Jackson integration knows how to serialize it. The `@Argument Map<String, Object> input` in the mutation resolver means Spring for GraphQL has already converted the GraphQL input type into a Java map.

**(6) Multiple URI template variables**
`.uri("/tasks/{taskId}/assign/{assigneeId}", taskId, assigneeId)` — positional substitution. The first variable is `taskId`, the second is `assigneeId`.

**(7) `.toBodilessEntity()`**
For DELETE operations that return no body (HTTP 204 No Content), `.body(SomeClass.class)` would throw because there's nothing to deserialize. `.toBodilessEntity()` returns a `ResponseEntity<Void>` and is the correct method for void responses.

---

### Step 4 — ActivityServiceClient

```java
// gateway-service/client/ActivityServiceClient.java

@Component
public class ActivityServiceClient {

    private final RestClient client;

    public ActivityServiceClient(
            @Qualifier("activityRestClient") RestClient client) {
        this.client = client;
    }

    public List<ActivityEventDto> getActivityByTask(String taskId) {
        return client.get()
                .uri("/activity/task/{taskId}", taskId)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public List<CommentDto> getCommentsByTask(String taskId) {
        return client.get()
                .uri("/comments/task/{taskId}", taskId)
                .retrieve()
                .body(new ParameterizedTypeReference<>() {});
    }

    public CommentDto addComment(String taskId, String userId, String body) {
        return client.post()
                .uri("/comments/task/{taskId}", taskId)
                .header("X-User-Id", userId)
                .body(Map.of("body", body))                    // (1)
                .retrieve()
                .body(CommentDto.class);
    }
}
```

**(1) `Map.of("body", body)`**
`Map.of` (Java 9+) creates an immutable map with one entry: `{ "body": "the comment text" }`. This becomes the JSON request body. It is the Go equivalent of `map[string]any{"body": body}` and the TypeScript equivalent of `{ body }`.

The activity-service and notification-service are grouped under separate client classes even though they all follow the same `RestClient` pattern. This is the **service client pattern**: one class per downstream service. The benefit is that if the activity-service changes its URL structure, you change one file, not all the places in resolvers where you'd otherwise call HTTP directly.

---

### Step 5 — GraphQlInterceptor

```java
// gateway-service/config/GraphQlInterceptor.java

@Component
public class GraphQlInterceptor implements WebGraphQlInterceptor {

    @Override
    public Mono<WebGraphQlResponse> intercept(
            WebGraphQlRequest request, Chain chain) {

        String userId = null;

        // Primary: extract from Spring Security context (JWT was validated by SecurityConfig)
        var auth = SecurityContextHolder.getContext().getAuthentication();
        if (auth != null && auth.getPrincipal() instanceof String principal) {  // (1)
            userId = principal;
        }

        // Fallback: check X-User-Id header (testing / service-to-service)
        if (userId == null) {
            var headerValues = request.getHeaders().get("X-User-Id");
            if (headerValues != null && !headerValues.isEmpty()) {
                userId = headerValues.getFirst();
            }
        }

        if (userId != null) {
            String finalUserId = userId;                                          // (2)
            request.configureExecutionInput((input, builder) ->
                    builder.graphQLContext(ctx ->
                            ctx.put("userId", finalUserId)).build());            // (3)
        }

        return chain.next(request);                                               // (4)
    }
}
```

This is the bridge between the HTTP/security world and the GraphQL execution world. Understanding this class is key to understanding how user identity flows through the entire request.

**(1) `auth.getPrincipal() instanceof String principal`**
Java 16+ pattern matching for `instanceof`. In one expression, this checks if `getPrincipal()` is a `String`, and if so, binds it to the variable `principal`. The equivalent pre-Java-16 code was:
```java
if (auth.getPrincipal() instanceof String) {
    principal = (String) auth.getPrincipal();
}
```
The `SecurityConfig` (ADR 03) configures JWT parsing to set the `principal` to the user ID string extracted from the token's subject claim. So `getPrincipal()` returns a `String` user ID after JWT validation.

**(2) `String finalUserId = userId`**
Lambda functions in Java can only capture variables that are effectively final (their value never changes after assignment). `userId` is re-assigned earlier in the method (potentially changed from null to a string), so it is not effectively final by the time the lambda is written. The `finalUserId` copy captures a value that won't change. This is a Java-specific restriction — Go closures and TypeScript closures capture the variable reference freely.

**(3) `builder.graphQLContext(ctx -> ctx.put("userId", finalUserId))`**
This is where the userId moves from the HTTP world into the GraphQL execution context. The `GraphQLContext` is a key-value map that travels with the GraphQL execution from the interceptor all the way down to every resolver method. Resolver methods access it via `DataFetchingEnvironment.getGraphQlContext()`.

The alternative — passing userId as a method argument through every call — would require changes to every resolver signature for every new piece of context. The context map is more flexible.

**(4) `return chain.next(request)`**
Like middleware in Go's `net/http`, Express, or Koa — call the next handler in the chain. Unlike a traditional servlet filter, this uses Project Reactor's `Mono` type because Spring for GraphQL's interceptor chain is built on Reactor even when the underlying server is servlet-based. You don't need to understand reactive programming to use this pattern — just know that `chain.next(request)` means "proceed with the modified request."

---

### Step 6 — QueryResolver

```java
// gateway-service/resolver/QueryResolver.java

@Controller                                                    // (1)
public class QueryResolver {

    private final TaskServiceClient taskClient;
    private final ActivityServiceClient activityClient;
    private final NotificationServiceClient notificationClient;

    public QueryResolver(TaskServiceClient taskClient,
                         ActivityServiceClient activityClient,
                         NotificationServiceClient notificationClient) {
        this.taskClient = taskClient;
        this.activityClient = activityClient;
        this.notificationClient = notificationClient;
    }

    @QueryMapping                                              // (2)
    public List<ProjectDto> myProjects(DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");  // (3)
        return taskClient.getMyProjects(userId);
    }

    @QueryMapping
    public ProjectDto project(@Argument String id,            // (4)
                               DataFetchingEnvironment env) {
        return taskClient.getProject(id);
    }

    @QueryMapping
    public List<ActivityEventDto> taskActivity(
            @Argument String taskId, DataFetchingEnvironment env) {
        return activityClient.getActivityByTask(taskId);
    }

    @QueryMapping
    public NotificationDto myNotifications(
            @Argument Boolean unreadOnly,                      // (5)
            DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        return notificationClient.getNotifications(userId, unreadOnly);
    }
}
```

**(1) `@Controller` not `@RestController`**
In Spring for GraphQL, resolver classes are annotated with `@Controller`, not `@RestController`. This is deliberate — GraphQL controllers do not handle HTTP directly. They handle GraphQL field resolution. The GraphQL runtime calls them when assembling the response, not the HTTP dispatch mechanism.

**(2) `@QueryMapping`**
Binds this method to the `myProjects` field in the `Query` type of the schema. Spring matches by method name: the method is `myProjects`, the schema field is `myProjects`. If the names differ, you can specify: `@QueryMapping("myProjects")`.

**(3) `env.getGraphQlContext().get("userId")`**
Reads the `userId` that `GraphQlInterceptor` placed into the context earlier. This is how the security context crosses from the interceptor to the resolver — there is no direct method call between them. The `userId` travels through the `GraphQLContext` map.

Notice that `project()` does not read the userId, but `myProjects()` does. `project(id: ID!)` fetches a specific project by ID — authorization is handled downstream (the task-service checks if the requesting user can access it). `myProjects` is user-scoped — the service needs the userId to filter to the requesting user's projects.

**(4) `@Argument String id`**
Binds the `id` argument from the GraphQL query:
```graphql
query {
  project(id: "proj-123") { name }
}
```
Spring for GraphQL maps the GraphQL argument `id` (String) to the Java method parameter `id` (String). For complex input types, `@Argument Map<String, Object> input` is used — Spring maps the GraphQL input object fields into a Java Map.

**(5) `@Argument Boolean unreadOnly`**
Using the boxed `Boolean` (not primitive `boolean`) because the schema defines `unreadOnly: Boolean` — nullable, optional argument. A primitive `boolean` would throw if the argument is not provided. A `Boolean` will be null if omitted, which the service client handles.

---

### Step 7 — MutationResolver

```java
// gateway-service/resolver/MutationResolver.java

@Controller
public class MutationResolver {

    // ... (constructor with same three clients)

    @MutationMapping
    public ProjectDto createProject(
            @Argument Map<String, Object> input,       // (1)
            DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        return taskClient.createProject(userId, input);
    }

    @MutationMapping
    public Boolean deleteProject(
            @Argument String id,
            DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        taskClient.deleteProject(id, userId);
        return true;                                   // (2)
    }

    @MutationMapping
    public TaskDto assignTask(
            @Argument String taskId,
            @Argument String userId,                   // (3)
            DataFetchingEnvironment env) {
        String requestingUserId = env.getGraphQlContext().get("userId");
        return taskClient.assignTask(taskId, userId, requestingUserId);
    }

    @MutationMapping
    public CommentDto addComment(
            @Argument String taskId,
            @Argument String body,
            DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        return activityClient.addComment(taskId, userId, body);
    }

    @MutationMapping
    public Boolean markNotificationRead(
            @Argument String id,
            DataFetchingEnvironment env) {
        String userId = env.getGraphQlContext().get("userId");
        notificationClient.markRead(userId, id);
        return true;
    }
}
```

**(1) `@Argument Map<String, Object> input`**
The schema defines `createProject(input: CreateProjectInput!)`. GraphQL input types are objects, not scalars. Spring for GraphQL maps the entire input object to a `Map<String, Object>` when you annotate with `@Argument`. Alternatively, you could define a Java class `CreateProjectInput` with `name` and `description` fields, annotated with nothing (Spring maps it automatically). The Map approach is chosen here to avoid creating DTO classes for every mutation input — the map is passed directly to the RestClient, which serializes it to JSON for the downstream service.

**(2) Returning `true` from a delete mutation**
The schema declares `deleteProject(id: ID!): Boolean!`. The task-service's delete endpoint returns HTTP 204 No Content. The gateway converts this to `true` — indicating success. If the delete throws an exception (not found, permission denied), Spring for GraphQL converts the exception to a GraphQL error before it reaches the client.

**(3) `@Argument String userId` in `assignTask`**
This is the target user (who is being assigned), passed as a GraphQL argument. The `requestingUserId` from the context is the authenticated user making the request (who is doing the assigning). Two different users, two different sources: one from the GraphQL argument, one from the security context. The downstream task-service receives both and can enforce authorization (e.g., only project owners can assign tasks).

---

### The Full Request Lifecycle

To make this concrete, here is the complete flow for `addComment(taskId: "task-123", body: "Looks good")`:

```
1. Frontend sends:
   POST /graphql
   Authorization: Bearer <jwt>
   { "query": "mutation { addComment(taskId: \"task-123\", body: \"LGTM\") { id body } }" }

2. Spring Security filter validates JWT → sets SecurityContext authentication
   principal = "user-456"

3. GraphQlInterceptor.intercept() runs:
   - Reads userId "user-456" from SecurityContextHolder
   - Puts userId into GraphQL context: { "userId": "user-456" }

4. Spring for GraphQL parses the query, finds "addComment" in Mutation type

5. MutationResolver.addComment() is called:
   - @Argument String taskId = "task-123"
   - @Argument String body = "LGTM"
   - env.getGraphQlContext().get("userId") = "user-456"

6. activityClient.addComment("task-123", "user-456", "LGTM") is called

7. RestClient sends:
   POST http://localhost:8082/comments/task/task-123
   X-User-Id: user-456
   { "body": "LGTM" }

8. activity-service saves comment, returns CommentDto JSON

9. RestClient deserializes to CommentDto

10. MutationResolver returns CommentDto

11. Spring for GraphQL serializes the requested fields { id body } to JSON

12. Response: { "data": { "addComment": { "id": "comment-789", "body": "LGTM" } } }
```

---

## Experiment

### 1. Enable GraphiQL in development

```yaml
# application.yml
spring:
  graphql:
    graphiql:
      enabled: true
```

Or use the environment variable: `GRAPHIQL_ENABLED=true`. Navigate to `http://localhost:8080/graphiql`. Try:
```graphql
query {
  myProjects {
    id
    name
  }
}
```
GraphiQL provides schema introspection, autocomplete, and inline documentation. This is one of the primary advantages of GraphQL over REST — the schema is self-documenting and explorable.

### 2. Add a RequestInterceptor to log all downstream calls

```java
@Bean("taskRestClient")
public RestClient taskServiceClient(@Value("${app.services.task-url}") String taskUrl) {
    return RestClient.builder()
            .baseUrl(taskUrl)
            .requestInterceptor((request, body, execution) -> {
                System.out.println(">>> " + request.getMethod() + " " + request.getURI());
                ClientHttpResponse response = execution.execute(request, body);
                System.out.println("<<< " + response.getStatusCode());
                return response;
            })
            .build();
}
```

This logs every outgoing HTTP request and its response status. Observe how many HTTP calls a single GraphQL query generates when it touches multiple services.

**Trade-off:** Each field resolver that calls a downstream service makes one HTTP request. A GraphQL query fetching task + activity + comments makes three HTTP requests. This is the N+1 problem at the gateway level — you'll see it clearly in the logs.

### 3. Add default error handling to RestClient

```java
@Bean("taskRestClient")
public RestClient taskServiceClient(@Value("${app.services.task-url}") String taskUrl) {
    return RestClient.builder()
            .baseUrl(taskUrl)
            .defaultStatusHandler(HttpStatusCode::is4xxClientError, (request, response) -> {
                throw new RuntimeException("Task service client error: " + response.getStatusCode());
            })
            .defaultStatusHandler(HttpStatusCode::is5xxServerError, (request, response) -> {
                throw new RuntimeException("Task service server error: " + response.getStatusCode());
            })
            .build();
}
```

By default, `RestClient` throws `HttpClientErrorException` for 4xx and `HttpServerErrorException` for 5xx. Custom handlers give you control over what exception type reaches your resolvers — and Spring for GraphQL will convert unhandled exceptions to GraphQL error responses.

### 4. Try a schema mismatch

Add a field to the schema that has no resolver method:
```graphql
type Query {
    nonExistentField: String   # add this
    ...existing fields...
}
```

Restart the gateway. Spring for GraphQL will start successfully but return null for `nonExistentField` (no resolver = null). Now add a resolver method for a field that doesn't exist in the schema:
```java
@QueryMapping
public String ghostField() { return "I don't exist in the schema"; }
```

Spring logs a warning but doesn't fail. This is the schema-first behavioral gap versus gqlgen (Go) which would not compile.

### 5. Add a context value in the interceptor and read it in a resolver

Extend `GraphQlInterceptor` to also extract the JWT's `email` claim and put it in context:
```java
ctx.put("userId", finalUserId)
ctx.put("userEmail", extractEmail(auth))  // add this
```

Then read it in a resolver:
```java
@QueryMapping
public UserDto me(DataFetchingEnvironment env) {
    String userId = env.getGraphQlContext().get("userId");
    String email = env.getGraphQlContext().get("userEmail");
    // use both
}
```

This demonstrates how the GraphQL context scales to carry multiple pieces of request-scoped data without changing every method signature.

---

## Check Your Understanding

1. **The `GraphQlInterceptor` has a fallback that reads `X-User-Id` from the HTTP header if no JWT is present.** When would this fallback be used in a real system? What are the security implications of trusting a raw header value? How would you remove this fallback in production?

2. **`@Controller` vs. `@RestController` is used for resolver classes.** If you accidentally annotate `QueryResolver` with `@RestController` and also add a `@QueryMapping` method, what do you think happens? Will the GraphQL routing work? Will the Spring MVC routing work? Try it and see.

3. **The `@Argument Map<String, Object> input` pattern is used for mutation inputs.** An alternative is defining a Java class `CreateTaskInput` with typed fields. Write out what that class would look like. What type-safety benefits do you get? What do you lose in terms of flexibility when the schema changes?

4. **`ParameterizedTypeReference<>(){}` is used when deserializing `List<ProjectDto>`.** Why can't you write `.body(List<ProjectDto>.class)`? What is Java type erasure, and why does it make generic type information unavailable at runtime?

5. **Look at the `assignTask` mutation resolver.** There are two user IDs in play: `userId` (the assignment target from the GraphQL argument) and `requestingUserId` (from the context). Trace both values through the entire stack: where does each come from? Where does each go? What would break if you mixed them up?

6. **The `deleteProject` mutation calls `taskClient.deleteProject()` which uses `.toBodilessEntity()`.** What would happen if you used `.body(Boolean.class)` instead? Write out the error message you'd expect and explain why it occurs.

7. **Currently, the gateway makes one HTTP request per downstream service call.** If a client sends a GraphQL query that requests `myProjects` (one call to task-service) and `myNotifications` (one call to notification-service), how many total HTTP requests does the gateway make? If the client's query also requests `taskActivity` for three tasks, how many total HTTP requests are made?

8. **`GraphQlInterceptor` uses `SecurityContextHolder.getContext().getAuthentication()` to read the security context.** Why does this work in a servlet (thread-per-request) application? Would this work in a Spring WebFlux (reactive) application? What is the reactive equivalent, and why does it differ?

9. **The schema defines `ActivityEvent.metadata` as `String` (serialized JSON) rather than a nested object type.** What would it look like to define a proper GraphQL type for event metadata given that different event types have different fields? Look up GraphQL `union` types and `interface` types. Which approach would you use, and what are the trade-offs?

10. **Compare the gateway pattern in this project to a REST proxy approach.** Write out the HTTP call sequence the frontend would need to make to replicate the data returned by this query without the gateway:**
    ```graphql
    query {
      task(id: "task-1") { id title status }
      taskActivity(taskId: "task-1") { eventType timestamp }
      taskComments(taskId: "task-1") { body authorId }
    }
    ```
    How many HTTP requests would the frontend need to make? What happens if the user's internet connection is slow?

# Password Authentication Addition

- **Date:** 2026-04-05
- **Status:** Accepted
- **Extends:** [ADR 03: Authentication and Security](java-task-management/03_authentication_and_security.md)

## Context

The Java Task Management system originally used Google OAuth 2.0 as the sole authentication method. This created two problems:

1. **Portfolio accessibility.** Visitors reviewing the portfolio must have a Google account and consent to OAuth to try the task manager. Recruiters or hiring managers who don't want to use Google login are blocked.
2. **Single point of failure.** If Google OAuth is misconfigured (wrong redirect URIs, consent screen in testing mode, expired credentials), the entire app is inaccessible. This happened during initial deployment — the "Access blocked: Authorization Error" screen appeared because the consent screen hadn't been published.

Adding password-based authentication lets visitors try the app with a throwaway email/password while keeping Google OAuth as a convenience option for those who prefer it.

## Decision

### Dual Auth with Shared JWT Infrastructure

Password auth reuses the existing JWT token system. Both auth methods call the same `issueTokens()` method, returning an identical `AuthResponse`:

```java
// Same response shape regardless of auth method
new AuthResponse(accessToken, refreshToken, userId, email, name, avatarUrl)
```

This means the frontend token handling, Apollo Client headers, and refresh flow work unchanged. No new token format, no conditional logic in the API layer.

### User Entity: Nullable Password Hash

Rather than separate tables for Google users and password users, a single `passwordHash` column was added to the `User` entity:

```java
@Column(name = "password_hash")
private String passwordHash; // null for Google-only users
```

A user who registers with a password has `passwordHash` set. A user who logs in via Google has it null. The login endpoint checks: if `passwordHash` is null or doesn't match, reject. This prevents Google-only users from being logged into via password guessing (there's no password to guess).

A second constructor disambiguates creation:

```java
public User(String email, String name, String avatarUrl) { ... }           // Google OAuth
public User(String email, String name, String passwordHash, boolean isPasswordUser) { ... } // Password
```

### BCrypt for Password Hashing

BCrypt via Spring Security's `BCryptPasswordEncoder` — already included in `spring-boot-starter-security`, no additional dependency.

**Why not Argon2?** Argon2 is the newer recommendation from OWASP, but it requires the Bouncy Castle dependency and is more complex to tune (memory, parallelism, iterations). For a portfolio project with low traffic, BCrypt's built-in work factor is sufficient and the zero-dependency advantage wins.

### Resend for Password Reset Emails

The forgot-password flow sends a reset link via [Resend](https://resend.com):

- **Free tier:** 100 emails/day (more than enough for a portfolio project)
- **Shared domain:** Sends from `onboarding@resend.dev`, avoiding the need to configure a custom domain or expose a personal Gmail
- **Simple SDK:** `com.resend:resend-java` — one dependency, builder-pattern API

**Alternatives considered:**

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| Gmail SMTP | No signup, 500/day | Exposes personal email, App Password management | Too personal |
| SendGrid | 100/day free, established | Heavier SDK, requires more config | Equivalent but more setup |
| **Resend** | 100/day free, modern API, shared domain | Newer service | **Chosen** — simplest integration |

### Reset Token Design

Password reset uses a UUID token stored in the database:

```java
@Entity
@Table(name = "password_reset_tokens")
public class PasswordResetToken {
    private String token;      // UUID string
    private User user;         // FK to users table
    private Instant expiresAt; // 1 hour from creation
}
```

**Why not JWT-based reset tokens?** A JWT reset token would be self-contained (no DB lookup needed), but:
- It can't be revoked once issued (no DB row to delete)
- It can't be single-use without a DB check anyway (to prevent replay)
- The simplicity of "generate UUID, store in DB, delete on use" beats the cleverness of stateless tokens for this use case

### Security: Don't Reveal Email Existence

The forgot-password endpoint always returns `204 No Content`, regardless of whether the email exists:

```java
public void forgotPassword(String email) {
    userRepository.findByEmail(email).ifPresent(user -> {
        // Only sends email if user exists, but always returns 204
        ...
    });
}
```

This prevents email enumeration attacks where an attacker probes the endpoint to discover which emails are registered.

### Frontend: View-Based State Machine

Instead of separate routes for login, register, and forgot-password, the `TasksPageContent` component manages an `AuthView` state:

```typescript
type AuthView = "login" | "register" | "forgot-password";
const [view, setView] = useState<AuthView>("login");
```

Each view renders its own form component. This keeps the URL stable at `/java/tasks` and avoids cluttering the router with auth-specific routes. The only separate route is `/java/tasks/reset-password` — because users arrive there from an email link with a token parameter.

### No Email Verification on Registration

Users can register and immediately log in without confirming their email. For a portfolio demo, the friction of email verification outweighs the benefit. If this were a production app with user-generated content or billing, email verification would be essential.

## Consequences

**Positive:**
- Portfolio is accessible to anyone with an email address
- Google OAuth remains available as a convenience
- No changes to existing JWT infrastructure, Apollo Client, or API authorization
- Password reset flow demonstrates real email integration (Resend)
- BCrypt + reset tokens follow OWASP recommendations

**Trade-offs:**
- New attack surface: password brute-forcing (mitigated by BCrypt's work factor, but no rate limiting yet)
- Resend free tier limits to 100 emails/day (sufficient for portfolio traffic)
- No email verification means users can register with fake emails (acceptable for a demo)
- The `isPasswordUser` boolean constructor parameter is awkward — it's a disambiguator, not a real field. A factory method or builder would be cleaner.

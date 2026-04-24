import { test, expect } from "@playwright/test";

// Java services go through the gateway at port 8080.
const GATEWAY_URL = process.env.SMOKE_API_URL || "http://localhost:8080";
const GRAPHQL_URL = `${GATEWAY_URL}/graphql`;

test.describe("Java compose-smoke CI tests", () => {
  // Shared state for cleanup
  let authCookieHeader: string = "";
  let projectId: string;
  let taskId: string;
  const testEmail = `ci-smoke-${Date.now()}@test.com`;
  const testPassword = "CiSmokeTest123!";

  test("gateway health check", async ({ request }) => {
    // Spring Boot gateway should respond on its base port
    const res = await request.get(`${GATEWAY_URL}/graphql`, {
      headers: { "Content-Type": "application/json" },
      data: { query: "{ __typename }" },
    });
    expect(res.ok(), "gateway GraphQL endpoint should respond").toBeTruthy();
  });

  test("GraphQL schema loads via introspection", async ({ request }) => {
    const res = await request.post(GRAPHQL_URL, {
      data: {
        query: `{ __schema { queryType { name } mutationType { name } } }`,
      },
    });
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(body.data.__schema.queryType.name).toBe("Query");
    expect(body.data.__schema.mutationType.name).toBe("Mutation");
  });

  test("register → project → task → verify activity", async ({ request }) => {
    // Step 1: Register
    const registerRes = await request.post(GRAPHQL_URL, {
      data: {
        query: `mutation Register($email: String!, $password: String!, $name: String!) {
          register(email: $email, password: $password, name: $name) { id email name }
        }`,
        variables: { email: testEmail, password: testPassword, name: "CI Smoke" },
      },
    });
    expect(registerRes.ok()).toBeTruthy();

    // Capture auth cookie from registration
    const regCookies = registerRes
      .headersArray()
      .filter((h) => h.name.toLowerCase() === "set-cookie")
      .map((h) => h.value);
    const regAccessCookie = regCookies.find((v) =>
      v.startsWith("access_token=")
    );
    if (regAccessCookie) {
      const match = regAccessCookie.match(/^access_token=([^;]+)/);
      if (match) authCookieHeader = `access_token=${match[1]}`;
    }

    const headers = {
      Cookie: authCookieHeader,
      "Content-Type": "application/json",
    };

    // Step 2: Create project
    const projectRes = await request.post(GRAPHQL_URL, {
      headers,
      data: {
        query: `mutation { createProject(input: { name: "CI Smoke Project", description: "Automated CI smoke test" }) { id name } }`,
      },
    });
    expect(projectRes.ok()).toBeTruthy();
    const projectBody = await projectRes.json();
    projectId = projectBody.data.createProject.id;
    expect(projectId).toBeTruthy();

    // Step 3: Create task
    const taskRes = await request.post(GRAPHQL_URL, {
      headers,
      data: {
        query: `mutation CreateTask($input: CreateTaskInput!) { createTask(input: $input) { id title } }`,
        variables: {
          input: {
            projectId,
            title: "CI Smoke Task",
            priority: "HIGH",
          },
        },
      },
    });
    expect(taskRes.ok()).toBeTruthy();
    const taskBody = await taskRes.json();
    taskId = taskBody.data.createTask.id;
    expect(taskId).toBeTruthy();

    // Step 4: Verify activity feed has entries (async — poll with retries)
    // Task creation triggers: task-service → RabbitMQ → notification-service → activity-service
    let activityFound = false;
    for (let i = 0; i < 20; i++) {
      const activityRes = await request.post(GRAPHQL_URL, {
        headers,
        data: {
          query: `query { taskActivity(taskId: "${taskId}") { id eventType } }`,
        },
      });
      if (activityRes.ok()) {
        const activityBody = await activityRes.json();
        if (
          activityBody.data?.taskActivity &&
          activityBody.data.taskActivity.length > 0
        ) {
          activityFound = true;
          break;
        }
      }
      await new Promise((r) => setTimeout(r, 500));
    }
    expect(
      activityFound,
      "activity feed should have entries after task creation (RabbitMQ async flow)"
    ).toBeTruthy();
  });

  test.afterAll(async ({ request }) => {
    if (!authCookieHeader) return;

    const headers = {
      Cookie: authCookieHeader,
      "Content-Type": "application/json",
    };

    if (taskId) {
      await request.post(GRAPHQL_URL, {
        headers,
        data: { query: `mutation { deleteTask(id: "${taskId}") }` },
      });
    }

    if (projectId) {
      await request.post(GRAPHQL_URL, {
        headers,
        data: { query: `mutation { deleteProject(id: "${projectId}") }` },
      });
    }

    await request.post(GRAPHQL_URL, {
      headers,
      data: { query: `mutation { deleteAccount }` },
    });
  });
});

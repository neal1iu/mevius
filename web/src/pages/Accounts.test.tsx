import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import Accounts from "./Accounts";
const json = (value: unknown) =>
  new Response(JSON.stringify(value), {
    headers: { "content-type": "application/json" },
  });
const catalog = {
  products: [
    {
      id: "fixture.product",
      provider_id: "fixture",
      resource_types: [{ id: "fixture.database", provider_id: "fixture" }],
    },
  ],
  relations: [],
};
function mount() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <Accounts />
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  window.history.replaceState({}, "", "/accounts");
  vi.restoreAllMocks();
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const path = String(input);
    if (path.endsWith("/catalog")) return json(catalog);
    if (path.endsWith("/provider-instances"))
      return json([
        { id: "instance", provider_id: "fixture", instance_key: "cloud" },
      ]);
    if (path.endsWith("/connections")) return json([]);
    if (path.endsWith("/connections/oauth/providers"))
      return json([
        {
          provider_id: "fixture",
          configured: true,
          configuration: { client_id: "public-client" },
        },
      ]);
    throw Error(path);
  });
});
it("authorizes a deployment independently of a scope", async () => {
  let payload: Record<string, unknown> | undefined;
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    const path = String(input);
    if (path.endsWith("/connections") && init?.method === "POST") {
      payload = JSON.parse(String(init.body));
      return json({ id: "connection" });
    }
    if (path.endsWith("/catalog")) return json(catalog);
    if (path.endsWith("/provider-instances"))
      return json([
        { id: "instance", provider_id: "fixture", instance_key: "cloud" },
      ]);
    if (path.endsWith("/connections")) return json([]);
    if (path.endsWith("/connections/oauth/providers")) return json([]);
    throw Error(path);
  });
  const user = userEvent.setup();
  mount();
  await screen.findByRole("option", { name: "fixture · cloud" });
  await user.selectOptions(
    screen.getByLabelText("Deployment instance"),
    "instance",
  );
  await user.type(screen.getByLabelText("Authorization label"), "My account");
  await user.type(
    screen.getByLabelText("Authorization token"),
    "authorization-secret",
  );
  await user.click(screen.getByRole("button", { name: "Save authorization" }));
  await waitFor(() =>
    expect(payload).toEqual({
      provider_instance_id: "instance",
      label: "My account",
      credential: "authorization-secret",
    }),
  );
  expect(payload).not.toHaveProperty("scope");
  expect(screen.getByLabelText("Authorization token")).toHaveValue("");
});
it("restores and consumes an OAuth session without rendering secrets", async () => {
  window.history.replaceState({}, "", "/accounts?oauth_session=session");
  let completed = false;
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    const path = String(input);
    if (path.endsWith("/connections/oauth/sessions/session"))
      return json({ status: "authorized" });
    if (
      path.endsWith("/connections/oauth/complete") &&
      init?.method === "POST"
    ) {
      completed = true;
      expect(JSON.parse(String(init.body))).toEqual({
        session_id: "session",
        label: "OAuth authorization",
      });
      return json({ id: "connection" });
    }
    if (path.endsWith("/catalog")) return json(catalog);
    if (path.endsWith("/provider-instances")) return json([]);
    if (path.endsWith("/connections")) return json([]);
    if (path.endsWith("/connections/oauth/providers")) return json([]);
    throw Error(path);
  });
  const user = userEvent.setup();
  mount();
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Save OAuth authorization" }),
    ).toBeEnabled(),
  );
  expect(screen.queryByText("access_token")).not.toBeInTheDocument();
  await user.click(
    screen.getByRole("button", { name: "Save OAuth authorization" }),
  );
  await waitFor(() => expect(completed).toBe(true));
});
it("shows independent OAuth application setup", async () => {
  const user = userEvent.setup();
  mount();
  await user.click(screen.getByText("OAuth application configuration"));
  expect(screen.getByLabelText("Client secret")).toHaveAttribute(
    "type",
    "password",
  );
  expect(
    await screen.findByRole("option", { name: "fixture · configured" }),
  ).toBeInTheDocument();
});

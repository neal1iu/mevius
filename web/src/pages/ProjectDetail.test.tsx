import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { expect, it, vi } from "vitest";
import ProjectDetail from "./ProjectDetail";
const json = (v: unknown) =>
  new Response(JSON.stringify(v), {
    headers: { "content-type": "application/json" },
  });
it("attaches the same resource again with a registered custom role and environment", async () => {
  let submitted: Record<string, unknown> | undefined;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const path = String(input);
    if (
      path.endsWith("/projects/project/resources") &&
      init?.method === "POST"
    ) {
      submitted = JSON.parse(String(init.body));
      return json({ id: "link2", ...submitted });
    }
    if (path.endsWith("/projects/project"))
      return json({
        project: { id: "project", name: "Demo" },
        resources: [
          {
            id: "existing",
            resource_id: "db",
            alias: "primary",
            role: "custom_database",
          },
        ],
      });
    if (path.endsWith("/resource-instances"))
      return json([
        {
          id: "db",
          resource_type_id: "fixture.database",
          display_name: "Database",
        },
      ]);
    if (path.endsWith("/catalog"))
      return json({
        products: [
          {
            resource_types: [
              { id: "fixture.database", compatible_roles: ["custom_database"] },
            ],
          },
        ],
        relations: [],
      });
    throw Error(path);
  });
  const user = userEvent.setup();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter initialEntries={["/projects/project"]}>
        <Routes>
          <Route path="/projects/:id" element={<ProjectDetail />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await screen.findByRole("option", { name: "Database" });
  await user.selectOptions(screen.getByLabelText("Resource"), "db");
  expect(
    screen.getByRole("option", { name: "custom_database" }),
  ).toBeInTheDocument();
  await user.type(screen.getByLabelText("Alias"), "analytics");
  await user.type(screen.getByLabelText("Environment label"), "sandbox");
  await user.click(screen.getByRole("button", { name: "Attach resource" }));
  await waitFor(() =>
    expect(submitted).toMatchObject({
      resource_id: "db",
      alias: "analytics",
      role: "custom_database",
      environment: "sandbox",
    }),
  );
  expect(
    screen.queryByRole("option", { name: "source" }),
  ).not.toBeInTheDocument();
});

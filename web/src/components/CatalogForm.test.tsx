import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { SchemaFields } from "./CatalogForm";
it("renders fields declared by an unfamiliar product schema", async () => {
  const onChange = vi.fn();
  const user = userEvent.setup();
  render(
    <SchemaFields
      schema={{
        version: 3,
        schema: {
          type: "object",
          properties: {
            retention_days: { type: "integer" },
            paused: { type: "boolean" },
            region: { type: "string", enum: ["eu", "us"] },
          },
          required: ["region"],
          additionalProperties: false,
        },
      }}
      value={{}}
      onChange={onChange}
    />,
  );
  await user.type(screen.getByLabelText("retention days"), "7");
  expect(onChange).toHaveBeenCalledWith({ retention_days: 7 });
  await user.click(screen.getByLabelText("paused"));
  expect(onChange).toHaveBeenCalledWith({ paused: true });
  await user.selectOptions(screen.getByLabelText("region *"), "eu");
  expect(onChange).toHaveBeenCalledWith({ region: "eu" });
});

import { render } from "@testing-library/react";
import { expect, test } from "vitest";
import { MarkdownText } from "./Markdown";

test("assistant markdown renders formatting, lists, links and tables", () => {
  const { container } = render(
    <MarkdownText text={"## Plan\n\n**Curry** and *rice*:\n\n- one\n- two\n\n[Recipe](https://example.com) `code`\n\n| a | b |\n|---|---|\n| 1 | 2 |"} />,
  );
  expect(container.querySelector("h2")).toHaveTextContent("Plan");
  expect(container.querySelector("strong")).toHaveTextContent("Curry");
  expect(container.querySelector("em")).toHaveTextContent("rice");
  expect(container.querySelectorAll("li")).toHaveLength(2);
  expect(container.querySelector("a")).toHaveAttribute("href", "https://example.com");
  expect(container.querySelector("a")).toHaveAttribute("rel", "noopener noreferrer");
  expect(container.querySelector("code")).toHaveTextContent("code");
  expect(container.querySelector(".md-table table td")).toHaveTextContent("1");
});

test("assistant markdown never renders raw HTML, script links or images", () => {
  const { container } = render(
    <MarkdownText text={'<b onclick="x()">hi</b><script>alert(1)</script>\n\n[click](javascript:alert(1))\n\n![a cat](https://example.com/cat.png)'} />,
  );
  expect(container.querySelector("b, script, img")).toBeNull();
  expect(container.querySelector("a")?.getAttribute("href") ?? "").not.toMatch(/javascript/i);
  expect(container).toHaveTextContent("a cat");
});

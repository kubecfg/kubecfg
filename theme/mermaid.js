// mdbook has no mermaid support: render ```mermaid code blocks in the browser.
import("https://cdn.jsdelivr.net/npm/mermaid@11.4.1/dist/mermaid.esm.min.mjs").then(({ default: mermaid }) => {
  const dark = ["coal", "navy", "ayu"].some((t) => document.documentElement.classList.contains(t));
  mermaid.initialize({ startOnLoad: false, theme: dark ? "dark" : "default" });
  const nodes = [...document.querySelectorAll("code.language-mermaid")].map((code) => {
    const div = document.createElement("div");
    div.className = "mermaid";
    div.textContent = code.textContent;
    code.parentElement.replaceWith(div);
    return div;
  });
  mermaid.run({ nodes });
});

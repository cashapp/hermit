function external_new_window(root = document) {
  root.querySelectorAll("a[href]").forEach((link) => {
    if (link.hostname && link.hostname !== location.hostname) {
      link.target = "_blank";
      link.rel = "noopener";
    }
  });
}

const COPY_PAGE_LABEL = "Copy Page";
const COPIED_LABEL = "Copied";
const COPY_RESET_DELAY_MS = 1600;
const COPY_PAGE_DENYLIST = new Set(["", "about", "sdks"]);
const COPY_ICON_SVG = '<svg stroke="currentColor" fill="none" stroke-width="1.9" viewBox="0 0 24 24" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" xmlns="http://www.w3.org/2000/svg"><rect x="9" y="9" width="11" height="11" rx="2"></rect><path d="M15 9V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h3"></path></svg>';
const CHECK_ICON_SVG = '<svg stroke="currentColor" fill="none" stroke-width="2" viewBox="0 0 24 24" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" height="12" width="12" xmlns="http://www.w3.org/2000/svg"><path d="m5 12.5 4 4 10-11"></path></svg>';

async function copyToClipboard(text) {
  if (!text) {
    return false;
  }
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch (_err) {}
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.select();
  const copied = document.execCommand("copy");
  textarea.remove();
  return copied;
}

function addCopyPageButton(root = document) {
  const article = root.querySelector("article.md-content__inner.md-typeset");
  if (!article || article.querySelector(".hermit-copy-page-btn")) {
    return;
  }
  const markdownNode = article.querySelector("#__hermit_page_markdown");
  const h1 = article.querySelector("h1");
  const pageUrl = markdownNode.dataset.pageUrl.replace(/^\/+|\/+$/g, "");
  if (COPY_PAGE_DENYLIST.has(pageUrl)) {
    return;
  }
  const pageTitle = h1.childNodes[0]?.textContent?.trim() || h1.textContent.trim();

  const button = document.createElement("button");
  button.type = "button";
  button.className = "md-button hermit-copy-page-btn";
  button.innerHTML = `<span class="hermit-copy-page-icon" aria-hidden="true">${COPY_ICON_SVG}</span><span class="hermit-copy-page-label">${COPY_PAGE_LABEL}</span>`;
  h1.classList.add("hermit-copy-page-title");
  h1.appendChild(button);

  let resetTimer;
  button.addEventListener("click", async () => {
    const markdown = JSON.parse(markdownNode.textContent).trim();
    const titleHeading = `# ${pageTitle}`;
    const content = markdown ? `${titleHeading}\n\n${markdown}` : titleHeading;
    if (!(await copyToClipboard(content))) {
      return;
    }
    button.classList.add("is-copied");
    button.querySelector(".hermit-copy-page-icon").innerHTML = CHECK_ICON_SVG;
    button.querySelector(".hermit-copy-page-label").textContent = COPIED_LABEL;
    window.clearTimeout(resetTimer);
    resetTimer = window.setTimeout(() => {
      button.classList.remove("is-copied");
      button.querySelector(".hermit-copy-page-icon").innerHTML = COPY_ICON_SVG;
      button.querySelector(".hermit-copy-page-label").textContent = COPY_PAGE_LABEL;
    }, COPY_RESET_DELAY_MS);
  });
}

function init(root = document) {
  external_new_window(root);
  addCopyPageButton(root);
}

document.addEventListener("DOMContentLoaded", () => init(document));
if (typeof document$ !== "undefined" && document$?.subscribe) {
  document$.subscribe(() => init(document));
}

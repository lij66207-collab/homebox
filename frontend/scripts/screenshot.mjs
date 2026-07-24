/* 截图工具：登录页 + 登录后物品页（如提供了账号密码环境变量） */
import { chromium } from "@playwright/test";

const base = process.env.BASE_URL || "http://localhost:7745";
const email = process.env.SHOT_EMAIL || "";
const password = process.env.SHOT_PASSWORD || "";
const tag = process.env.SHOT_TAG || "shot";

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });

await page.goto(base + "/", { waitUntil: "networkidle" });
await page.screenshot({ path: `/tmp/${tag}-login.png` });

if (email && password) {
  await page.fill("#login-username", email);
  await page.fill("#login-password", password);
  await page.click('button[type="submit"]');
  await page.waitForTimeout(4000);
  await page.waitForLoadState("networkidle").catch(() => {});
  await page.screenshot({ path: `/tmp/${tag}-home.png` });

  await page.goto(base + "/items", { waitUntil: "networkidle" });
  await page.waitForTimeout(1500);
  await page.screenshot({ path: `/tmp/${tag}-items.png` });
}

await browser.close();
console.log("done");

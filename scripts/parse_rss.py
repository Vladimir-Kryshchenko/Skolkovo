#!/usr/bin/env python3
import xml.etree.ElementTree as ET
import json
import sys

root = ET.parse("/tmp/rss.xml").getroot()
items = root.findall(".//item")

CATS = {
    "legislative_acts": "Нормативные акты",
    "design_rules": "Правила проектирования",
    "development": "Развитие",
    "other": "Прочее",
    "tenders": "Закупки",
    "unactual_documents": "Устаревшие",
    "anti_corruption": "Антикоррупция",
    "cybersec_and_persdata": "Кибербезопасность",
}

docs = []
for item in items:
    title = item.find("title").text or ""
    link = item.find("link").text or ""
    pubdate = item.find("pubDate").text or ""
    desc = item.find("description").text or ""
    is_outdated = "\u0423\u0422\u0420\u0410\u0422\u0418\u041b" in title.upper() or "\u0423\u0422\u0420\u0410\u0422\u0418\u041b\u041e" in title.upper()
    status = "\u0423\u0441\u0442\u0430\u0440\u0435\u043b" if is_outdated else "\u0414\u0435\u0439\u0441\u0442\u0432\u0443\u0435\u0442"
    icon = "\u274c" if is_outdated else "\u2705"
    cat = "\u0414\u043e\u043a\u0443\u043c\u0435\u043d\u0442\u044b"
    for slug, name in CATS.items():
        if slug in link:
            cat = name
            break
    doc_id = link.split("/")[-1].replace(".aspx", "") if link else ""
    docs.append({
        "id": doc_id, "title": title, "link": link,
        "pubdate": pubdate, "status": status, "category": cat,
    })
    print(f"  {icon} [{status}] [{cat}]")
    print(f"    {title[:120]}")
    print(f"    {link}")
    print(f"    {pubdate}")
    print()

print(f"\n\u0412\u0441\u0435\u0433\u043e: {len(docs)}")
print(f"\u0414\u0435\u0439\u0441\u0442\u0432\u0443\u0435\u0442: {sum(1 for d in docs if d['status'] == '\u0414\u0435\u0439\u0441\u0442\u0432\u0443\u0435\u0442')}")
print(f"\u0423\u0441\u0442\u0430\u0440\u0435\u043b: {sum(1 for d in docs if d['status'] == '\u0423\u0441\u0442\u0430\u0440\u0435\u043b')}")

# Group by category
print("\n" + "="*60)
for cat_name in CATS.values():
    cat_docs = [d for d in docs if d["category"] == cat_name]
    if cat_docs:
        print(f"\n\U0001f4c1 {cat_name}: {len(cat_docs)} \u0434\u043e\u043a\u0443\u043c\u0435\u043d\u0442\u043e\u0432")
        for d in cat_docs:
            icon = "\u2705" if d["status"] == "\u0414\u0435\u0439\u0441\u0442\u0432\u0443\u0435\u0442" else "\u274c"
            print(f"  {icon} {d['title'][:80]}")

with open("/tmp/skolkovo_docs.json", "w") as f:
    json.dump(docs, f, ensure_ascii=False, indent=2)
print(f"\n\u0421\u043e\u0445\u0440\u0430\u043d\u0435\u043d\u043e \u0432 /tmp/skolkovo_docs.json")

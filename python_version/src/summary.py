import json
from collections import Counter

with open("friends.json", mode="r", encoding="UTF-8") as f:
    all_friends: list[dict] = json.load(f)

status_counter = Counter()
for friend in all_friends:
    info = friend.get("info")
    status = info.get("status", "OK")
    status_counter[status] += 1

print(status_counter)

with open("memories.json", mode="r", encoding="UTF-8") as f:
    all_memories: list[dict] = json.load(f)

blackboard_activities = []
normal_activities = []

for memory in all_memories:
    if memory.get("question"):
        blackboard_activities.append(memory)
    else:
        normal_activities.append(memory)

print(f"一共{len(all_memories)}条动态\n黑板墙：{len(blackboard_activities)}条\n动态板：{len(normal_activities)}条")

with open("blackboard_memories.json", mode="w", encoding="UTF-8") as f:
    json.dump(blackboard_activities, f, ensure_ascii=False, indent=2)

with open("normal_memories.json", mode="w", encoding="UTF-8") as f:
    json.dump(normal_activities, f, ensure_ascii=False, indent=2)

refined_blackboard_memories = []
for memory in blackboard_activities:
    memo = {"question": memory["question"]["content"], "my_answer": memory["content"], "images": memory["images"],
            "time": memory["created_at"]}
    refined_blackboard_memories.append(memo)
with open("refined_blackboard_memories.json", mode="w", encoding="UTF-8") as f:
    json.dump(refined_blackboard_memories, f, ensure_ascii=False, indent=2)


refined_normal_memories = []
for memory in normal_activities:
    memo = {"title": memory.get("title"), "content": memory["content"], "images": memory["images"],
            "time": memory["created_at"]}
    if memo.get("outchain"):
        memo["outlink"] = {memory["outchain"], memory["outchain_title"], memory["outchain_img"], memory["outchain_author"]}
    refined_normal_memories.append(memo)
with open("refined_normal_memories.json", mode="w", encoding="UTF-8") as f:
    json.dump(refined_normal_memories, f, ensure_ascii=False, indent=2)


with open("memories_for_llm.md",encoding="UTF-8",mode="w") as f:
    f.write("## 我在某社交软件上发过的所有动态\n\n")
    for m in refined_normal_memories:
        f.write(f"### 时间:{m['time']}\n\n")
        f.write(f"{m['content']}\n\n")
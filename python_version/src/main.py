from robust_http_client import RobustHTTPClient
import json
import time
import logging

# --- 日志记录器基础配置 ---
# 建议在您的主程序入口处配置，这里为了演示方便放在此处
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)

client=RobustHTTPClient(5,3,20)

auth_headers={"authorization":"aUkkLKuspKmAZAaYAj2sXmRL","user-agent":"okhttp/4.12.0","accept-encoding":"gzip"}



def get_friend_profile(user_id:str):
    return client.get(url=f"https://imsummer.cn/api/v9/users/{user_id}",headers=auth_headers)

def get_all_friend_info():
    all_friends_raw = client.get(url="https://imsummer.cn/api/v9/user/relationships", headers=auth_headers)
    all_friends=[]
    count=1
    for friend in all_friends_raw["friends"]:
        new_friend=friend
        new_friend["info"]=get_friend_profile(friend["id"])
        all_friends.append(new_friend)
        logging.info(f"第{count}个好友保存完成")
        count+=1
        time.sleep(1)
    with open("friends.json",mode="w",encoding="utf-8") as f:
        json.dump(all_friends,f,ensure_ascii=False,indent=2,)

def get_memory(limit:int,offset:int):
    time.sleep(1)
    return client.get(f"https://imsummer.cn/api/v9/user/activities?limit={limit}&offset={offset}",headers=auth_headers)

def get_all_memory():
    limit=200
    offset=0

    all_memory=[]

    while True:
        page=get_memory(limit,offset)
        if not page:
            break
        else:
            all_memory+=page
            offset+=limit

    logging.info(f"一共{len(all_memory)}条动态")
    with open("memories.json",mode="w",encoding="utf-8") as f:
        json.dump(all_memory,f,ensure_ascii=False,indent=2)

get_all_memory()


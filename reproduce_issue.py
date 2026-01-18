import requests
import time

url = "http://localhost:8080"

print("Sending 10 requests...")
for i in range(1, 11):
    try:
        response = requests.get(url)
        print(f"Request {i}: Status {response.status_code} - {response.text.strip()}")
    except Exception as e:
        print(f"Request {i}: Failed - {e}")
    time.sleep(0.1)

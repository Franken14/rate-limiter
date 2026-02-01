import requests
import time
import subprocess
import os

def check_server(url):
    try:
        response = requests.get(url)
        return response.status_code
    except requests.exceptions.ConnectionError:
        return 0

def main():
    print("starting server with local redis...")
    # Assume redis is running on localhost:6379 
    # we can't easily stop redis from here without potentially affecting other things, 
    # but we can configure the app to look at a wrong port to simulate downtime.
    
    # Actually, let's just assume the user will stop redis manually or we can point to a wrong port.
    # For this script, we will just hit the endpoint and report status.
    
    url = "http://localhost:8080"
    
    print(f"Sending request to {url}")
    status = check_server(url)
    print(f"Status Code: {status}")

    if status == 500: # Internal Server Error often when Redis fails in current code
        print("Received 500 - System failed closed (as expected before fix)")
    elif status == 200:
        print("Received 200 - Request successful")
    elif status == 429:
        print("Received 429 - Rate limit exceeded")
    else:
        print(f"Received {status}")

if __name__ == "__main__":
    main()

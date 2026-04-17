import json
import socket
import sys
import yaml

# Get certificate of authority
ELECTION_PATH = '/run/electionDaemon/election.sock'
client_socket = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)

try:
    # Connect to the server
    client_socket.connect(ELECTION_PATH)
    print(f"Connecting to {ELECTION_PATH}")
except Exception as e:
    print(e)
    sys.exit(-2)

coa = json.loads(client_socket.recv(409600).decode())
if len(coa['endorsements']) == 0:
    print("Cannot shutdown, not leader")
    sys.exit(-4)

# Create a Unix Domain Socket client
SOCKET_PATH = '/run/commsDaemon/comms.sock'
client_socket = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)

try:
    # Connect to the server
    client_socket.connect(SOCKET_PATH)
    print(f"Connecting to {SOCKET_PATH}")
except Exception as e:
    print(e)

msgBody = {"source":"123e4567-e89b-12d3-a456-426614174001", "msg":"","msg_type":"Shutdown"}
msgBody['authority'] = coa
client_socket.sendall(json.dumps(msgBody).encode('utf-8'))
import time
import requests
import multiprocessing
from multiprocessing import Process
import os
import tomli
from fabric import ThreadingGroup as Group
from fabric import Connection

user = os.getenv('APIR_USER')
password = os.getenv('APIR_PASSWORD')
simul_dir = '/' + user + '/go/src/github.com/si-co/vpir-code/simulations/multi/'
default_server_command = "screen -dm ./server -scheme={} -dbLen={} -elemBitSize={} -nRows={} -blockLen={} && sleep 15"

def test_command():
    return 'uname -a'

def load_config(file):
    with open(file, 'rb') as f:
        return tomli.load(f)

def load_servers_config():
    return load_config('config.toml')

def load_general_config():
    return load_config('simul.toml')

def load_individual_config(config_file):
    return load_config(config_file)

def servers_addresses():
    addrs = []
    config = load_servers_config()
    for s in config["servers"].values():
        addrs.append(s['ip'])

    return addrs

def client_address():
    config = load_servers_config()
    return config["client"]

def servers_pool():
    servers = servers_addresses()
    return Group(*servers, 
            user=user, 
            connect_kwargs={'password': password,},
            )

def server_setup(c, sid):
    # enable agent forwarding for git pull
    c.forward_agent = True
    with c.cd(simul_dir), c.prefix('PATH=$PATH:/usr/local/go/bin'):
        c.run('git pull', warn=True)
        c.run('echo '+ str(sid) + ' > server/sid')
        c.run('bash ' + 'setup.sh')
    # disable now useless agent forwarding
    c.forward_agent = False

def client_setup(c):
    # enable agent forwarding for git pull
    c.forward_agent = True
    with c.cd(simul_dir), c.prefix('PATH=$PATH:/usr/local/go/bin'):
        c.run('git pull', warn = True)
        c.run('bash ' + 'setup.sh')
    # disable now useless agent forwarding
    c.forward_agent = False

def server_pir_classic_command(dbLen, elemBitSize, nRows, blockLen):
    return default_server_command.format('pir-classic', dbLen, elemBitSize, nRows, blockLen)

def server_pir_merkle_command(dbLen, elemBitSize, nRows, blockLen):
   return default_server_command.format('pir-merkle', dbLen, elemBitSize, nRows, blockLen)

def experiment_pir_classic(server_pool, client):
    print('Experiment PIR classic')
    gc = load_general_config()
    ic = load_individual_config('pirClassic.toml')
    print("\t Run", len(server_pool), "servers")
    server_pool.run('cd ' + simul_dir + 'server && ' + server_pir_classic_command(gc['DBBitLengths'][0], ic['ElementBitSize'], ic['NumRows'], ic['BlockLength']))
    time.sleep(10)
    print("\t Run", len(server_pool), "client")
    client.run('cd ' + simul_dir + 'client && ./client')

print("Servers' setup")
pool = servers_pool()
for i, c in enumerate(pool):
    server_setup(c, i)
print("Client's setup")
client_host = client_address()
client = Connection(client_host, user=user, connect_kwargs={'password': password,})
client_setup(client)
experiment_pir_classic(pool, client)

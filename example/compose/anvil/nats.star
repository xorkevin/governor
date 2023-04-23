load("anvil:std", "workflow", "os", "json", "crypto", "template")


def readauth():
    auth = json.unmarshal(os.readmodfile("natsauth.json"))
    return {
        "username": auth["username"],
        "passhash": crypto.bcrypt(auth["password"], 10),
    }


def gennatsconf(auth):
    tpl = os.readmodfile("nats.conf.tmpl")
    os.writefile("output/nats.conf", template.gotpl(tpl, auth))


def main(args):
    auth = workflow.execactivity(readauth)
    workflow.execactivity(gennatsconf, auth)

# logspout-l2met

A [Logspout](https://github.com/gliderlabs/logspout) adapter for writing Docker container logs to [l2met](https://github.com/ryandotsmith/l2met).

## usage
After you created your custom Logspout build, you can just run it as:
```console
$ 	docker run --rm \
		-e LOGSPOUT=ignore \
		--name="logspout" \
		--volume=/var/run/docker.sock:/var/run/docker.sock \
		mycompany/logspout l2met://
```

You will also need to set the environment variable `L2MET_URL` with your l2met url.
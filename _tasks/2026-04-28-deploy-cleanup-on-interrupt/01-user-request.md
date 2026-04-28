# User Request

A minor bug: when a user interrupts the deployment by pressing ctrl+c while the
deployment is waiting for the health endpoint to come up, the subsequent
deployment will fail because the existing docker container won't be cleaned up.
I deleted it manually and everything worked so it's minor, but please fix.

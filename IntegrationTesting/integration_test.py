import logging
import subprocess
import unittest
import shlex
import mock_listener
from subprocess import Popen, PIPE


# Set debug prints
import time

debug = True

# Set logger
logging.basicConfig(format='%(asctime)s %(name)-12s %(levelname)-8s %(message)s', level=logging.INFO)
logger = logging.getLogger(__name__)


# should match makefile name
plugin_name = "logzio:latest"


def debug(message):
    if debug:
        logger.info(str(message))


def subprocess_cmd(command):
    debug("Command : {}".format(command))
    ret_val = subprocess.call(command, shell=True)
    debug("return value is {}".format(ret_val))
    return ret_val


def get_exitcode_stdout_stderr(cmd):
    """
    Execute the external command and get its exitcode, stdout and stderr.
    """
    debug("Command : {}".format(cmd))
    args = shlex.split(cmd)

    proc = Popen(args, stdout=PIPE, stderr=PIPE)
    out, err = proc.communicate()
    exitcode = proc.returncode
    debug("return value : {}".format(out))
    #
    return exitcode, out, err


def delete_image(image):
    return subprocess_cmd('sudo docker rmi {}'.format(image))


def get_url(token):
    return "https://listener.logz.io:8071/?token={}".format(token)


# to add mock and change password
class TestDockerDriver(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        # logzio_url = "{0}/?token={1}".format("https://listener.logz.io:8071", "sCvFJmZlhxgCwSnRglhmXoIDpMxkMOlr")
        assert subprocess_cmd('cd ..; sudo make all') == 0, "Failed to create plugin"
        assert subprocess_cmd('sudo docker plugin enable {}'.format(plugin_name)) == 0, "Failed to enable plugin"
        assert subprocess_cmd('sudo service docker restart') == 0, "Failed to enable plugin"

    def _test_one_container(self):
        self.assertTrue(subprocess_cmd("sudo docker build -t test_one_container:latest --build-arg iterations=100"
                                       " --build-arg prefix=test --build-arg time=0 .") == 0,
                        "Fail to build test_one_container image")
        self.assertTrue(subprocess_cmd("sudo docker run --log-driver=logzio"
                                       " --log-opt logzio-token=sCvFJmZlhxgCwSnRglhmXoIDpMxkMOlr"
                                       " --log-opt logzio-url=https://listener.logz.io:8071"
                                       " --log-opt logzio-dir-path=./test_one "
                                       "test_one_container") == 0,
                        "Fail to run test_one_container container")

        # test ReadLogs
        exitcode, docker_id, err = get_exitcode_stdout_stderr('sudo docker ps -l -q')
        self.assertTrue(exitcode == 0, "Failed to find latest container, from test_one_container - {}".format(err))
        exitcode, logs, err = get_exitcode_stdout_stderr('sudo docker logs {}'.format(docker_id))
        self.assertTrue(exitcode == 0, "Failed to show latest logs {}".format(err))
        idx = 0
        for log in logs.splitlines():
            self.assertIn("test_{}".format(idx), log)
            idx += 1

        # check queue location
        exitcode, path, err = \
            get_exitcode_stdout_stderr('sudo find /var/lib/docker/plugins/ -name test -type d')
        self.assertTrue(exitcode == 0, "Failed to find queue dir - {}".format(err))

        # add check logs at mock

    def _test_multi_containers_same_logger(self):
        num_containers = 5
        for i in xrange(num_containers):
            self.assertTrue(subprocess_cmd("sudo docker build -t test_multi_containers_same_logger{0}:latest"
                                           " --build-arg iterations=10"
                                           " --build-arg prefix=multi_test_containers_same_logger{0} "
                                           "--build-arg time=3 .".format(i)) == 0,
                            "Failed to build image test_multi_containers_same_logger{}".format(i))
            self.assertTrue(subprocess_cmd("sudo docker run -td --log-driver=logzio"
                                           " --log-opt logzio-token=sCvFJmZlhxgCwSnRglhmXoIDpMxkMOlr"
                                           " --log-opt logzio-url=https://listener.logz.io:8071"
                                           " --log-opt logzio-dir-path=./test_multi_same "
                                           "test_multi_containers_same_logger{}".format(i)) == 0,
                            "Fail to run test_multi_containers_same_logger container number {}".format(i))

        time.sleep(60)
        # test ReadLogs
        exitcode, dockers_id, err = get_exitcode_stdout_stderr('sudo docker ps -n {} -q'.format(num_containers))
        self.assertTrue(exitcode == 0, "Failed to find latest container - {}".format(err))
        containers_id_list = dockers_id.splitlines()
        self.assertEqual(len(containers_id_list), num_containers)

        for container_number, docker_id in enumerate(containers_id_list):
            exitcode, logs, err = get_exitcode_stdout_stderr('sudo docker logs {}'.format(docker_id))
            self.assertTrue(exitcode == 0, "Failed to show latest logs for container {0} - {1}".format(dockers_id, err))

            idx = 0
            for log in logs.splitlines():
                self.assertIn("multi_test_containers_same_logger{0}_{1}"
                              .format(num_containers - container_number - 1, idx)
                              , log)
                idx += 1

        # check queue location
        exitcode, path, err = \
            get_exitcode_stdout_stderr('sudo find /var/lib/docker/plugins/ -name test_multi_same -type d')
        self.assertTrue(exitcode == 0, "Failed to find queue dir - {}".format(err))

        # add check logs at mock

    def _test_kill_container(self):
        self.assertTrue(subprocess_cmd("sudo docker build -t test_kill_container:latest"
                                       " --build-arg iterations=10000"
                                       " --build-arg prefix=test_kill_container "
                                       "--build-arg time=0 .") == 0,
                        "Failed to build image test_kill_container")
        self.assertTrue(subprocess_cmd("sudo docker run -td --log-driver=logzio"
                                       " --log-opt logzio-token=sCvFJmZlhxgCwSnRglhmXoIDpMxkMOlr"
                                       " --log-opt logzio-url=https://listener.logz.io:8071"
                                       " --log-opt logzio-dir-path=./test_kill_container "
                                       "test_kill_container") == 0,
                        "Failed to run test_kill_container container ")

        exitcode, docker_name, err = get_exitcode_stdout_stderr('sudo docker ps -l --format "{{.Names}}"')
        self.assertTrue(exitcode == 0, "Failed to find latest container, from test_kill_container - {}".format(err))
        self.assertTrue(subprocess_cmd('sudo docker rm -f {0}'.format(docker_name)) == 0,
                        "Failed to kill docker {}".format(docker_name))

        # check queue location
        exitcode, path, err = \
            get_exitcode_stdout_stderr('sudo find /var/lib/docker/plugins/ -name test_kill_container -type d')
        self.assertTrue(exitcode == 0, "Failed to find queue dir - {}".format(err))

        # add check logs at mock

    def _test_multi_containers_different_logger(self):
        mock = mock_listener.MockLogzioListener()
        num_containers = 5
        for i in xrange(num_containers):
            self.assertTrue(subprocess_cmd("sudo docker build -t test_multi_containers_different_logger{0}:latest"
                                           " --build-arg iterations=10"
                                           " --build-arg prefix=test_multi_containers_different_logger{0} "
                                           "--build-arg time=3 .".format(i)) == 0,
                            "Failed to build image test_multi_containers_different_logger{}".format(i))
            self.assertTrue(subprocess_cmd("sudo docker run -td --log-driver=logzio"
                                           " --log-opt logzio-token={0}"
                                           " --log-opt logzio-url=https://listener.logz.io:8071"
                                           " --log-opt logzio-dir-path=./test_multi_containers_different_logger "
                                           "test_multi_containers_different_logger{0}".format(i)) == 0,
                            "Failed to run test_multi_containers_different_logger container number {}".format(i))

        time.sleep(60)
        # test ReadLogs
        exitcode, dockers_id, err = get_exitcode_stdout_stderr('sudo docker ps -n {} -q'.format(num_containers))
        self.assertTrue(exitcode == 0, "Failed to find latest containers - {}".format(err))
        containers_id_list = dockers_id.splitlines()
        self.assertEqual(len(containers_id_list), num_containers)

        for container_number, docker_id in enumerate(containers_id_list):
            exitcode, logs, err = get_exitcode_stdout_stderr('sudo docker logs {}'.format(docker_id))
            self.assertTrue(exitcode == 0, "Failed to show latest logs for container {0} - {1}".format(dockers_id, err))

            idx = 0
            for log in logs.splitlines():
                self.assertIn("test_multi_containers_different_logger{0}_{1}"
                              .format(num_containers - container_number - 1, idx)
                              , log)
                idx += 1

                self.assertTrue(mock.find_log(log), "Failed to find {} in the mock listener".format(log))

        # check queue location
        exitcode, path, err = \
            get_exitcode_stdout_stderr('sudo find /var/lib/docker/plugins/ -name test_multi_containers_different_logger'
                                       ' -type d')
        self.assertTrue(exitcode == 0, "Failed to find queue dir - {}".format(err))

    def _test_daemon_global_configuration(self):
        self.assertTrue(subprocess_cmd("sudo docker build -t test_daemon_global_configuration:latest"
                                       " --build-arg iterations=100"
                                       " --build-arg prefix=test_kill_container "
                                       "--build-arg time=0 .") == 0,
                        "Failed to build image test_deamon_global_configuration")
        self.assertTrue(subprocess_cmd('sudo cp ./daemon.json /etc/docker/') == 0,
                        "Failed to copy daemon config file")
        self.assertTrue(subprocess_cmd('sudo service docker restart') == 0,
                        "Failed to restart docker service")
        self.assertTrue(subprocess_cmd("sudo docker run test_daemon_global_configuration:latest") == 0,
                        "Failed to run test_daemon_global_configuration container")
        # check queue location
        exitcode, path, err = \
            get_exitcode_stdout_stderr('sudo find /var/lib/docker/plugins/ '
                                       '-name test_daemon_global_configuration -type d')
        self.assertTrue(exitcode == 0, "Failed to find queue dir - {}".format(err))

        # add check logs at mock

        self.assertTrue(subprocess_cmd('sudo rm /etc/docker/daemon.json') == 0,
                        "Failed to delete daemon config file")


if __name__ == '__main__':
    unittest.main()

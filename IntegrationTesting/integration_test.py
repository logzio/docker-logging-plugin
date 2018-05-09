import json
import logging
import subprocess
import unittest
import shlex

import os

import mock_listener
from subprocess import Popen, PIPE


# Set debug prints
import time

_debug = True

# Set logger
logging.basicConfig(format='%(asctime)s %(name)-12s %(levelname)-8s %(message)s', level=logging.INFO)
logger = logging.getLogger(__name__)


# should match makefile name
plugin_name = "logzio/logzio-logging-plugin:latest"


def debug(message):
    if _debug:
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


def remove_image(image):
    subprocess_cmd('sudo docker rmi {}'.format(image))


def remove_containers():
    subprocess_cmd('sudo docker rm $(sudo docker ps -a -q)')


def cleanup(images):
    remove_containers()
    for image in images:
        remove_image(image)


# to add mock and change password
class TestDockerDriver(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        subprocess_cmd('cd ..; sudo make all')
        subprocess_cmd('sudo docker plugin set {} LOGZIO_DRIVER_LOGS_DRAIN_TIMEOUT=1s'.format(plugin_name))
        subprocess_cmd('sudo docker plugin enable {}'.format(plugin_name))
        subprocess_cmd('sudo service docker restart')

    @classmethod
    def tearDownClass(cls):
        subprocess_cmd('cd ..; sudo make clean')

    def setUp(self):
        # TODO - change to github
        self.logzio_listener = mock_listener.MockLogzioListener()
        self.logzio_listener.clear_logs_buffer()
        self.logzio_listener.clear_server_error()
        self.url = "http://{0}:{1}".format(self.logzio_listener.get_host(), self.logzio_listener.get_port())

    def test_one_container(self):
        self.assertTrue(subprocess_cmd("sudo docker build -t test_one_container:latest --build-arg iterations=100"
                                       " --build-arg prefix=test --build-arg time=0 .") == 0
                        , "Fail to build test_one_container image")
        self.assertTrue(subprocess_cmd("sudo docker run --log-driver={} --log-opt logzio-token=token "
                                       "--log-opt logzio-url={} "
                                       "--log-opt logzio-dir-path=./test_one "
                                       "test_one_container".format(plugin_name, self.url)) == 0,
                        "Fail to run test_one_container container")
        time.sleep(1)
        # test ReadLogs
        exitcode, docker_id, err = get_exitcode_stdout_stderr('sudo docker ps -l -q')
        self.assertTrue(exitcode == 0, "Failed to find latest container, from test_one_container - {}".format(err))
        exitcode, logs, err = get_exitcode_stdout_stderr('sudo docker logs {}'.format(docker_id))
        self.assertTrue(exitcode == 0, "Failed to show latest logs {}".format(err))
        idx = 0
        for log in logs.splitlines():
            self.assertIn("test_{}".format(idx), log)
            self.assertTrue(self.logzio_listener.find_log(log),
                            "Failed to find {} in the mock listener".format(log))
            idx += 1

        # check queue location
        exitcode, path, err = \
            get_exitcode_stdout_stderr('sudo find /var/lib/docker/plugins/ -name test_one -type d')
        self.assertTrue(exitcode == 0, "Failed to find queue dir - {}".format(err))
        cleanup(["test_one_container"])

    def _test_multi_containers_same_logger(self):
        num_containers = 1
        for i in xrange(num_containers):
            self.assertTrue(subprocess_cmd("sudo docker build -t test_multi_containers_same_logger{0}:latest"
                                           " --build-arg iterations=10"
                                           " --build-arg prefix=multi_test_containers_same_logger{0} "
                                           "--build-arg time=3 .".format(i)) == 0,
                            "Failed to build image test_multi_containers_same_logger{}".format(i))
            self.assertTrue(subprocess_cmd("sudo docker run -td --log-driver={0}"
                                           " --log-opt logzio-token=token"
                                           " --log-opt logzio-url={1}"
                                           " --log-opt logzio-dir-path=./test_multi_same "
                                           "test_multi_containers_same_logger{2}".format(plugin_name, self.url, i)) == 0
                            , "Failed to run test_multi_containers_same_logger container number {}".format(i))

        time.sleep(40)
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
                              .format(num_containers - container_number - 1, idx), log)
                self.assertTrue(self.logzio_listener.find_log(log),
                                "Failed to find {} in the mock listener".format(log))
                idx += 1

        # check queue location
        exitcode, path, err = \
            get_exitcode_stdout_stderr('sudo find /var/lib/docker/plugins/ -name test_multi_same -type d')
        self.assertTrue(exitcode == 0, "Failed to find queue dir - {}".format(err))

        cleanup(["test_multi_containers_same_logger{0}".format(i) for i in xrange(num_containers)])

    def test_kill_container(self):
        self.assertTrue(subprocess_cmd("sudo docker build -t test_kill_container:latest"
                                       " --build-arg iterations=10000"
                                       " --build-arg prefix=test_kill_container "
                                       "--build-arg time=0 .") == 0,
                        "Failed to build image test_kill_container")
        self.assertTrue(subprocess_cmd("sudo docker run -td --log-driver={}"
                                       " --log-opt logzio-token=token"
                                       " --log-opt logzio-url={}"
                                       " --log-opt logzio-dir-path=./test_kill_container "
                                       "test_kill_container".format(plugin_name, self.url)) == 0,
                        "Failed to run test_kill_container container ")

        exitcode, docker_name, err = get_exitcode_stdout_stderr('sudo docker ps -l --format "{{.Names}}"')
        self.assertTrue(exitcode == 0, "Failed to find latest container, from test_kill_container - {}".format(err))
        self.assertTrue(subprocess_cmd('sudo docker rm -f {0}'.format(docker_name)) == 0,
                        "Failed to kill docker {}".format(docker_name))

        # check queue location
        exitcode, path, err = \
            get_exitcode_stdout_stderr('sudo find /var/lib/docker/plugins/ -name test_kill_container -type d')
        self.assertTrue(exitcode == 0, "Failed to find queue dir - {}".format(err))

        cleanup(["test_kill_container"])

    def test_multi_containers_different_logger(self):
        num_containers = 5
        for i in xrange(num_containers):
            self.assertTrue(subprocess_cmd("sudo docker build -t test_multi_containers_different_logger{0}:latest"
                                           " --build-arg iterations=10"
                                           " --build-arg prefix=test_multi_containers_different_logger{0} "
                                           "--build-arg time=3 .".format(i)) == 0,
                            "Failed to build image test_multi_containers_different_logger{}".format(i))
            self.assertTrue(subprocess_cmd("sudo docker run -td --log-driver={0}"
                                           " --log-opt logzio-token=token{1}"
                                           " --log-opt logzio-url={2}"
                                           " --log-opt logzio-dir-path=./test_multi_containers_different_logger "
                                           "test_multi_containers_different_logger{1}"
                                           .format(plugin_name, i, self.url)) == 0,
                            "Failed to run test_multi_containers_different_logger container number {}"
                            .format(i))

        time.sleep(40)
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
                              .format(num_containers - container_number - 1, idx), log)
                self.assertTrue(self.logzio_listener.find_log(log),
                                "Failed to find {} in the mock listener".format(log))
                idx += 1

        # check queue location
        exitcode, path, err = \
            get_exitcode_stdout_stderr('sudo find /var/lib/docker/plugins/ -name test_multi_containers_different_logger'
                                       ' -type d')
        self.assertTrue(exitcode == 0, "Failed to find queue dir - {}".format(err))
        cleanup(["test_multi_containers_different_logger{0}".format(i) for i in xrange(num_containers)])

    def test_daemon_global_configuration(self):
        self.assertTrue(subprocess_cmd("sudo docker build -t test_daemon_global_configuration:latest"
                                       " --build-arg iterations=100"
                                       " --build-arg prefix=test_daemon_global_configuration "
                                       "--build-arg time=0 --build-arg multiline=False .") == 0,
                        "Failed to build image test_deamon_global_configuration")
        import io
        try:
            to_unicode = unicode
        except NameError:
            to_unicode = str

        with io.open('daemon.json', 'w', encoding='utf8') as outfile:
            str_ = json.dumps({
                                  "log-driver": "{}".format(plugin_name),
                                  "log-opts": {
                                    "logzio-token": "token",
                                    "logzio-url": "{}".format(self.url),
                                    "logzio-dir-path": "./test_deamon_global_configuration"
                                    }
                              }, indent=4, sort_keys=True, separators=(',', ': '), ensure_ascii=False)
            outfile.write(to_unicode(str_))

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

        self.assertTrue(subprocess_cmd('sudo rm /etc/docker/daemon.json') == 0,
                        "Failed to delete daemon config file")
        # test ReadLogs
        exitcode, docker_id, err = get_exitcode_stdout_stderr('sudo docker ps -l -q')
        self.assertTrue(exitcode == 0, "Failed to find latest container, test_daemon_global_configuration - {}"
                        .format(err))
        exitcode, logs, err = get_exitcode_stdout_stderr('sudo docker logs {}'.format(docker_id))
        self.assertTrue(exitcode == 0, "Failed to show latest logs {}".format(err))

        time.sleep(1)
        idx = 0
        for log in logs.splitlines():
            self.assertIn("test_daemon_global_configuration_{}".format(idx), log)
            self.assertTrue(self.logzio_listener.find_log(log),
                            "Failed to find {} in the mock listener".format(log))
            idx += 1

        cleanup(["test_daemon_global_configuration"])


if __name__ == '__main__':
    unittest.main()

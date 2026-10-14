# -*- coding: utf-8 -*-
"""
PSM+ 唤醒验证脚本
订阅 wendao/+/psm/up 主题，观察设备是否每5分钟上报一条唤醒消息
用法: python psm_monitor.py
"""
import sys
import time

try:
    import paho.mqtt.client as mqtt
except ImportError:
    print("请先安装依赖: pip install paho-mqtt")
    sys.exit(1)

SERVER_ADDR = "panel.wendaoiot.com"
SERVER_PORT = 1883

def on_connect(client, userdata, flags, rc):
    if rc == 0:
        print("[{}] 已连接服务器，开始监听...".format(time.strftime("%H:%M:%S")))
        client.subscribe("wendao/+/psm/up", qos=0)
    else:
        print("连接失败, rc={}".format(rc))
        sys.exit(1)

def on_message(client, userdata, msg):
    ts = time.strftime("%Y-%m-%d %H:%M:%S")
    print("\n[{}] 收到唤醒上报!".format(ts))
    print("  topic  : {}".format(msg.topic))
    print("  payload: {}".format(msg.payload.decode("utf-8", "ignore")))
    print("  >>> 如果 wakeup_reason=1 (dtimer)，说明定时唤醒正常 <<<")

def main():
    client = mqtt.Client(client_id="psm_monitor_" + str(int(time.time())))
    client.on_connect = on_connect
    client.on_message = on_message
    print("正在连接 {}:{} ...".format(SERVER_ADDR, SERVER_PORT))
    try:
        client.connect(SERVER_ADDR, SERVER_PORT, keepalive=60)
        client.loop_forever()
    except KeyboardInterrupt:
        print("\n退出")
    except Exception as e:
        print("连接错误: {}".format(e))
        sys.exit(1)

if __name__ == "__main__":
    main()

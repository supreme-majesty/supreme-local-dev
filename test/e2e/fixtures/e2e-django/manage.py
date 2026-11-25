#!/usr/bin/env python
# Proves Django detection (manage.py runserver 0.0.0.0:8000), pip install into the venv and routing.
import os
import sys

if __name__ == "__main__":
    os.environ.setdefault("DJANGO_SETTINGS_MODULE", "e2eproj.settings")
    from django.core.management import execute_from_command_line

    execute_from_command_line(sys.argv)

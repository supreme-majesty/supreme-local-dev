import os

from django.http import HttpResponse
from django.urls import path


def index(request):
    return HttpResponse(f"E2E-DJANGO-OK venv={os.environ.get('VIRTUAL_ENV')} uid={os.getuid()}\n")


urlpatterns = [path("", index)]

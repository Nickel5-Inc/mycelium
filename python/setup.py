from setuptools import setup, find_packages

setup(
    name="mycelium",
    version="0.1.0",
    packages=find_packages(),
    install_requires=[
        "substrate-interface>=1.7.10",
        "py-sr25519-bindings>=0.2.1",
        "py-ed25519-zebra-bindings>=1.1.0",
        "py-bip39-bindings>=0.1.12",
        "base58>=2.1.1",
        "cryptography>=43.0.0",
        "pycryptodome>=3.21.0",
    ],
    extras_require={
        "full": [
            "numpy>=2.1.2",
            "pandas>=2.2.3",
            "torch>=2.5.1",
        ]
    },
    author="Mycelium Contributors",
    author_email="",
    description="A Python library for interacting with the Mycelium network",
    long_description=open("README.md").read(),
    long_description_content_type="text/markdown",
    url="https://github.com/yourusername/mycelium",
    classifiers=[
        "Development Status :: 3 - Alpha",
        "Intended Audience :: Developers",
        "License :: OSI Approved :: MIT License",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.8",
        "Programming Language :: Python :: 3.9",
        "Programming Language :: Python :: 3.10",
    ],
    python_requires=">=3.8",
) 
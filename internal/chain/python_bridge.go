package chain

/*
#cgo pkg-config: python3
#cgo LDFLAGS: -L/opt/homebrew/opt/python@3.13/Frameworks/Python.framework/Versions/3.13/lib -lpython3.13
#define Py_LIMITED_API
#include <Python.h>

static PyObject* _init_substrate_interface(const char* url) {
	PyObject *pModule, *pFunc, *pArgs, *pValue;

	// Import the substrate_bridge module
	pModule = PyImport_ImportModule("substrate_bridge");
	if (pModule == NULL) {
		PyErr_Print();
		return NULL;
	}

	// Get the init_substrate_interface function
	pFunc = PyObject_GetAttrString(pModule, "init_substrate_interface");
	if (pFunc == NULL) {
		Py_DECREF(pModule);
		PyErr_Print();
		return NULL;
	}

	// Create argument tuple
	pArgs = PyTuple_New(1);
	pValue = PyBytes_FromString(url);
	PyTuple_SetItem(pArgs, 0, pValue);

	// Call the function
	pValue = PyObject_CallObject(pFunc, pArgs);

	Py_DECREF(pArgs);
	Py_DECREF(pFunc);
	Py_DECREF(pModule);

	return pValue;
}

static PyObject* _verify_signature(PyObject* substrate, const char* hotkey, const char* signature, const char* message) {
	PyObject *pModule, *pFunc, *pArgs, *pValue;

	pModule = PyImport_ImportModule("substrate_bridge");
	if (pModule == NULL) {
		PyErr_Print();
		return NULL;
	}

	pFunc = PyObject_GetAttrString(pModule, "verify_signature");
	if (pFunc == NULL) {
		Py_DECREF(pModule);
		PyErr_Print();
		return NULL;
	}

	pArgs = PyTuple_New(4);
	PyTuple_SetItem(pArgs, 0, substrate);
	PyTuple_SetItem(pArgs, 1, PyBytes_FromString(hotkey));
	PyTuple_SetItem(pArgs, 2, PyBytes_FromString(signature));
	PyTuple_SetItem(pArgs, 3, PyBytes_FromString(message));

	pValue = PyObject_CallObject(pFunc, pArgs);

	Py_DECREF(pArgs);
	Py_DECREF(pFunc);
	Py_DECREF(pModule);

	return pValue;
}

static PyObject* _get_stake(PyObject* substrate, const char* hotkey, const char* coldkey) {
	PyObject *pModule, *pFunc, *pArgs, *pValue;

	pModule = PyImport_ImportModule("substrate_bridge");
	if (pModule == NULL) {
		PyErr_Print();
		return NULL;
	}

	pFunc = PyObject_GetAttrString(pModule, "get_stake");
	if (pFunc == NULL) {
		Py_DECREF(pModule);
		PyErr_Print();
		return NULL;
	}

	pArgs = PyTuple_New(3);
	PyTuple_SetItem(pArgs, 0, substrate);
	PyTuple_SetItem(pArgs, 1, PyBytes_FromString(hotkey));
	PyTuple_SetItem(pArgs, 2, PyBytes_FromString(coldkey));

	pValue = PyObject_CallObject(pFunc, pArgs);

	Py_DECREF(pArgs);
	Py_DECREF(pFunc);
	Py_DECREF(pModule);

	return pValue;
}

static PyObject* _serve_axon(PyObject* substrate, const char* hotkey, unsigned int ip, unsigned short port, unsigned short netuid) {
	PyObject *pModule, *pFunc, *pArgs, *pValue;

	pModule = PyImport_ImportModule("substrate_bridge");
	if (pModule == NULL) {
		PyErr_Print();
		return NULL;
	}

	pFunc = PyObject_GetAttrString(pModule, "serve_axon");
	if (pFunc == NULL) {
		Py_DECREF(pModule);
		PyErr_Print();
		return NULL;
	}

	pArgs = PyTuple_New(5);
	PyTuple_SetItem(pArgs, 0, substrate);
	PyTuple_SetItem(pArgs, 1, PyBytes_FromString(hotkey));
	PyTuple_SetItem(pArgs, 2, PyLong_FromUnsignedLong(ip));
	PyTuple_SetItem(pArgs, 3, PyLong_FromUnsignedLong(port));
	PyTuple_SetItem(pArgs, 4, PyLong_FromUnsignedLong(netuid));

	pValue = PyObject_CallObject(pFunc, pArgs);

	Py_DECREF(pArgs);
	Py_DECREF(pFunc);
	Py_DECREF(pModule);

	return pValue;
}
*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"
)

var (
	pyLock    sync.Mutex
	substrate *C.PyObject
)

// InitPythonBridge initializes the Python interpreter and substrate interface
func InitPythonBridge(wsURL string) error {
	pyLock.Lock()
	defer pyLock.Unlock()

	if C.Py_IsInitialized() == 0 {
		C.Py_Initialize()
	}

	cURL := C.CString(wsURL)
	defer C.free(unsafe.Pointer(cURL))

	substrate = C._init_substrate_interface(cURL)
	if substrate == nil {
		return fmt.Errorf("failed to initialize substrate interface")
	}

	return nil
}

// VerifySignature verifies a signature using the Python substrate interface
func VerifySignature(hotkey, signature, message string) (bool, error) {
	pyLock.Lock()
	defer pyLock.Unlock()

	cHotkey := C.CString(hotkey)
	cSignature := C.CString(signature)
	cMessage := C.CString(message)
	defer C.free(unsafe.Pointer(cHotkey))
	defer C.free(unsafe.Pointer(cSignature))
	defer C.free(unsafe.Pointer(cMessage))

	result := C._verify_signature(substrate, cHotkey, cSignature, cMessage)
	if result == nil {
		return false, fmt.Errorf("signature verification failed")
	}
	defer C.Py_DecRef(result)

	isValid := C.PyObject_IsTrue(result) == 1
	return isValid, nil
}

// GetStake gets the staked amount for a validator
func GetStake(hotkey, coldkey string) (float64, error) {
	pyLock.Lock()
	defer pyLock.Unlock()

	cHotkey := C.CString(hotkey)
	cColdkey := C.CString(coldkey)
	defer C.free(unsafe.Pointer(cHotkey))
	defer C.free(unsafe.Pointer(cColdkey))

	result := C._get_stake(substrate, cHotkey, cColdkey)
	if result == nil {
		return 0, fmt.Errorf("failed to get stake")
	}
	defer C.Py_DecRef(result)

	stake := C.PyFloat_AsDouble(result)
	if stake == -1.0 && C.PyErr_Occurred() != nil {
		return 0, fmt.Errorf("invalid stake value")
	}

	return float64(stake), nil
}

// ServeAxon submits validator information to the chain
func ServeAxon(hotkey string, ip uint32, port uint16, netuid uint16) error {
	pyLock.Lock()
	defer pyLock.Unlock()

	cHotkey := C.CString(hotkey)
	defer C.free(unsafe.Pointer(cHotkey))

	result := C._serve_axon(substrate, cHotkey, C.uint(ip), C.ushort(port), C.ushort(netuid))
	if result == nil {
		return fmt.Errorf("failed to serve axon")
	}
	defer C.Py_DecRef(result)

	return nil
}

// Cleanup releases Python resources
func Cleanup() {
	pyLock.Lock()
	defer pyLock.Unlock()

	if substrate != nil {
		C.Py_DecRef(substrate)
	}
	C.Py_Finalize()
}

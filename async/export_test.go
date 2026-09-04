package async

// SetAfterBeginAdmissionForTest sets a test hook executed in enqueue right after beginAdmission.
func (r *Runtime) SetAfterBeginAdmissionForTest(hook func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.afterBeginAdmissionForTest = hook
}

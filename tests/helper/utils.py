def expect_success(func):
    def wrapper(*args, **kwargs):
        try:
            func(*args, **kwargs)
        except:
            raise Exception("The action was expected to pass, but it failed")
    return wrapper

def expect_failure(func):
    def wrapper(*args, **kwargs):
        try:
            func(*args, **kwargs)
            raise Exception("The action was expected to fail, but it passed")
        except:
            return 0       
    return wrapper
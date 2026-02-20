import time

def expect_success(func):
    def wrapper(*args, **kwargs):
        try:
            func(*args, **kwargs)
        except:
            raise Exception("The transaction/action was expected to pass, but it failed")
    return wrapper

def expect_success_within_retries(func, retry_count=3, wait_in_seconds=60):
    def wrapper(*args, **kwargs):
        last_exc = None
        for attempt in range(retry_count + 1):
            try:
                func(*args, **kwargs)
                return
            except Exception as e:
                last_exc = e
                if attempt < retry_count:
                    time.sleep(wait_in_seconds)
        raise Exception(
            f"The transaction/action was expected to pass, but it failed after "
            f"{retry_count + 1} attempt(s). Last error: {last_exc}"
        )
    return wrapper


def expect_failure(func):
    def wrapper(*args, **kwargs):
        try:
            func(*args, **kwargs)
            raise Exception("The transaction/action was expected to fail, but it passed")
        except:
            return 0
    return wrapper

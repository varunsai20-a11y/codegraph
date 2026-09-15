class UserService:
    def get_user(self, user_id: str):
        return {"id": user_id, "name": "Alice"}

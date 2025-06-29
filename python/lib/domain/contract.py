from abc import ABCMeta, abstractmethod, abstractproperty

class Contract():
    __metaclass__=ABCMeta

    @abstractmethod
    def Ping(self):
        pass

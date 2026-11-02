package certificate

import "github.com/sirupsen/logrus"

func (s *Service) log(level logrus.Level, bucket, domain, stage, message string) {
	if s.Logger != nil {
		s.Logger.WithFields(logrus.Fields{"bucket": bucket, "domain": domain, "stage": stage}).Log(level, message)
	}
}
